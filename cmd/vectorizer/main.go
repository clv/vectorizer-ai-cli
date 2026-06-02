package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL = "https://api.vectorizer.ai/api/v1"

	headerImageToken = "X-Image-Token"
	headerReceipt    = "X-Receipt"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type config struct {
	apiID     string
	apiSecret string
	apiURL    string
	timeout   time.Duration
}

type repeatedFlag []string

func (f *repeatedFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *repeatedFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, command, commandArgs, err := parseGlobal(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	var runErr error
	switch command {
	case "":
		printUsage(stderr)
		return 2
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "vectorizer %s (%s, %s)\n", version, commit, date)
		return 0
	case "account":
		runErr = account(cfg, commandArgs, stdout)
	case "vectorize":
		runErr = vectorize(cfg, commandArgs, stdout, stderr)
	case "download":
		runErr = download(cfg, commandArgs, stdout, stderr)
	case "delete":
		runErr = deleteImage(cfg, commandArgs, stdout)
	default:
		runErr = fmt.Errorf("unknown command %q", command)
	}

	if runErr != nil {
		fmt.Fprintln(stderr, "error:", runErr)
		return 1
	}
	return 0
}

func parseGlobal(args []string, stderr io.Writer) (config, string, []string, error) {
	cfg := config{
		apiID:     os.Getenv("VECTORIZER_API_ID"),
		apiSecret: os.Getenv("VECTORIZER_API_SECRET"),
		apiURL:    envOrDefault("VECTORIZER_API_URL", defaultAPIURL),
		timeout:   10 * time.Minute,
	}

	fs := flag.NewFlagSet("vectorizer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.apiID, "api-id", cfg.apiID, "Vectorizer.AI API Id. Defaults to VECTORIZER_API_ID.")
	fs.StringVar(&cfg.apiSecret, "api-secret", cfg.apiSecret, "Vectorizer.AI API Secret. Defaults to VECTORIZER_API_SECRET.")
	fs.StringVar(&cfg.apiURL, "api-url", cfg.apiURL, "Base API URL. Defaults to VECTORIZER_API_URL or the production API.")
	fs.DurationVar(&cfg.timeout, "timeout", cfg.timeout, "HTTP timeout, for example 30s, 2m, or 10m.")
	fs.Usage = func() { printUsage(stderr) }

	if err := fs.Parse(args); err != nil {
		return cfg, "", nil, err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return cfg, "", nil, nil
	}
	return cfg, rest[0], rest[1:], nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func requireAuth(cfg config) error {
	if strings.TrimSpace(cfg.apiID) == "" {
		return errors.New("missing API Id; set VECTORIZER_API_ID or pass --api-id before the command")
	}
	if strings.TrimSpace(cfg.apiSecret) == "" {
		return errors.New("missing API Secret; set VECTORIZER_API_SECRET or pass --api-secret before the command")
	}
	return nil
}

func account(cfg config, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("account", flag.ContinueOnError)
	raw := fs.Bool("raw", false, "Print raw JSON without pretty formatting.")
	rest, err := parseCommandFlags(fs, args, map[string]bool{})
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errors.New("account does not accept positional arguments")
	}
	if err := requireAuth(cfg); err != nil {
		return err
	}

	body, headers, err := doRequest(cfg, http.MethodGet, "/account", nil, "", "")
	if err != nil {
		return err
	}
	printResponseHeaders(headers, io.Discard)
	if *raw {
		_, err = stdout.Write(body)
		return err
	}
	return writeJSON(stdout, body)
}

func vectorize(cfg config, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("vectorize", flag.ContinueOnError)
	output := fs.String("o", "", "Output file path, or '-' for stdout. Required.")
	outputLong := fs.String("output", "", "Output file path, or '-' for stdout. Required.")
	imageURL := fs.String("url", "", "URL to fetch as the input image.")
	imageBase64 := fs.String("base64", "", "Base64-encoded input image.")
	imageToken := fs.String("token", "", "Retained Image Token to re-vectorize.")
	format := fs.String("format", "", "Output format, for example svg, pdf, eps, dxf, or png.")
	mode := fs.String("mode", "", "API mode, for example production, preview, test, or test_preview.")
	retentionDays := fs.Int("retention-days", -1, "Retain the image/result for this many days.")
	maxPixels := fs.Int("max-pixels", -1, "Maximum input pixel count before resizing.")
	var params repeatedFlag
	fs.Var(&params, "param", "Additional literal API form field as key=value. May be repeated.")

	rest, err := parseCommandFlags(fs, args, map[string]bool{
		"o": true, "output": true, "url": true, "base64": true, "token": true,
		"format": true, "mode": true, "retention-days": true, "max-pixels": true,
		"param": true,
	})
	if err != nil {
		return err
	}
	out := firstNonEmpty(*outputLong, *output)
	inputPath := ""
	if len(rest) > 1 {
		return errors.New("vectorize accepts at most one input file")
	}
	if len(rest) == 1 {
		inputPath = rest[0]
	}
	if out == "" {
		return errors.New("missing output path; pass -o FILE or --output FILE")
	}
	if err := requireAuth(cfg); err != nil {
		return err
	}

	sourceCount := countNonEmpty(inputPath, *imageURL, *imageBase64, *imageToken)
	if sourceCount != 1 {
		return errors.New("provide exactly one image source: input file, --url, --base64, or --token")
	}

	fields, err := parseParams(params)
	if err != nil {
		return err
	}
	setIfNotEmpty(fields, "image.url", *imageURL)
	setIfNotEmpty(fields, "image.base64", *imageBase64)
	setIfNotEmpty(fields, "image.token", *imageToken)
	setIfNotEmpty(fields, "output.file_format", *format)
	setIfNotEmpty(fields, "mode", *mode)
	setIfNonNegative(fields, "policy.retention_days", *retentionDays)
	setIfNonNegative(fields, "input.max_pixels", *maxPixels)

	body, headers, err := doRequest(cfg, http.MethodPost, "/vectorize", fields, "image", inputPath)
	if err != nil {
		return err
	}
	if err := writeBinary(out, body, stdout); err != nil {
		return err
	}
	printResponseHeaders(headers, stderr)
	return nil
}

func download(cfg config, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	output := fs.String("o", "", "Output file path, or '-' for stdout. Required.")
	outputLong := fs.String("output", "", "Output file path, or '-' for stdout. Required.")
	format := fs.String("format", "", "Output format, for example svg, pdf, eps, dxf, or png.")
	receipt := fs.String("receipt", "", "Receipt returned in X-Receipt when upgrading a preview result.")
	var params repeatedFlag
	fs.Var(&params, "param", "Additional literal API form field as key=value. May be repeated.")

	rest, err := parseCommandFlags(fs, args, map[string]bool{
		"o": true, "output": true, "format": true, "receipt": true, "param": true,
	})
	if err != nil {
		return err
	}
	out := firstNonEmpty(*outputLong, *output)
	if out == "" {
		return errors.New("missing output path; pass -o FILE or --output FILE")
	}
	if len(rest) != 1 {
		return errors.New("download requires exactly one IMAGE_TOKEN argument")
	}
	if err := requireAuth(cfg); err != nil {
		return err
	}

	fields, err := parseParams(params)
	if err != nil {
		return err
	}
	fields["image.token"] = rest[0]
	setIfNotEmpty(fields, "output.file_format", *format)
	setIfNotEmpty(fields, "receipt", *receipt)

	body, headers, err := doRequest(cfg, http.MethodPost, "/download", fields, "", "")
	if err != nil {
		return err
	}
	if err := writeBinary(out, body, stdout); err != nil {
		return err
	}
	printResponseHeaders(headers, stderr)
	return nil
}

func deleteImage(cfg config, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	raw := fs.Bool("raw", false, "Print raw JSON without pretty formatting.")
	rest, err := parseCommandFlags(fs, args, map[string]bool{})
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("delete requires exactly one IMAGE_TOKEN argument")
	}
	if err := requireAuth(cfg); err != nil {
		return err
	}
	fields := map[string]string{"image.token": rest[0]}
	body, _, err := doRequest(cfg, http.MethodPost, "/delete", fields, "", "")
	if err != nil {
		return err
	}
	if *raw {
		_, err = stdout.Write(body)
		return err
	}
	return writeJSON(stdout, body)
}

func doRequest(cfg config, method, path string, fields map[string]string, fileField, filePath string) ([]byte, http.Header, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	req, err := newRequest(ctx, cfg.apiURL, method, path, fields, fileField, filePath)
	if err != nil {
		return nil, nil, err
	}
	req.SetBasicAuth(cfg.apiID, cfg.apiSecret)
	req.Header.Set("User-Agent", "Vectorizer.AI CLI/"+version)
	if method == http.MethodGet {
		req.Header.Set("Accept", "application/json")
	} else if path == "/delete" {
		req.Header.Set("Accept", "application/json")
	} else {
		req.Header.Set("Accept", "image/svg+xml, application/postscript, application/pdf, application/dxf, image/png, application/json")
	}

	client := &http.Client{Timeout: cfg.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, nil, apiError(resp.StatusCode, body)
	}
	return body, resp.Header, nil
}

func newRequest(ctx context.Context, baseURL, method, path string, fields map[string]string, fileField, filePath string) (*http.Request, error) {
	url := strings.TrimRight(baseURL, "/") + path
	if method == http.MethodGet {
		return http.NewRequestWithContext(ctx, method, url, nil)
	}

	if filePath == "" {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writeFields(writer, fields); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, url, &body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return req, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	go func() {
		defer file.Close()

		if err := writeFields(writer, fields); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		part, err := writer.CreateFormFile(fileField, filepath.Base(filePath))
		if err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		if err := writer.Close(); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		_ = pipeWriter.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, method, url, pipeReader)
	if err != nil {
		file.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func writeFields(writer *multipart.Writer, fields map[string]string) error {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := writer.WriteField(key, fields[key]); err != nil {
			return err
		}
	}
	return nil
}

func parseParams(params repeatedFlag) (map[string]string, error) {
	fields := map[string]string{}
	for _, param := range params {
		key, value, ok := strings.Cut(param, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --param %q; expected key=value", param)
		}
		fields[key] = value
	}
	return fields, nil
}

func setIfNotEmpty(fields map[string]string, key, value string) {
	if value != "" {
		fields[key] = value
	}
}

func setIfNonNegative(fields map[string]string, key string, value int) {
	if value >= 0 {
		fields[key] = strconv.Itoa(value)
	}
}

func parseCommandFlags(fs *flag.FlagSet, args []string, valueFlags map[string]bool) ([]string, error) {
	flagArgs := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !isFlagArg(arg) {
			positional = append(positional, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		name := flagArgName(arg)
		if strings.Contains(arg, "=") || !valueFlags[name] {
			continue
		}
		if i+1 >= len(args) {
			break
		}
		i++
		flagArgs = append(flagArgs, args[i])
	}

	if err := fs.Parse(append(flagArgs, positional...)); err != nil {
		return nil, err
	}
	return fs.Args(), nil
}

func isFlagArg(arg string) bool {
	return arg != "-" && strings.HasPrefix(arg, "-")
}

func flagArgName(arg string) string {
	arg = strings.TrimLeft(arg, "-")
	name, _, _ := strings.Cut(arg, "=")
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func countNonEmpty(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func writeBinary(path string, body []byte, stdout io.Writer) error {
	if path == "-" {
		_, err := stdout.Write(body)
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func writeJSON(out io.Writer, body []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		_, writeErr := out.Write(body)
		return writeErr
	}
	pretty.WriteByte('\n')
	_, err := out.Write(pretty.Bytes())
	return err
}

func printResponseHeaders(headers http.Header, out io.Writer) {
	for _, header := range []string{headerImageToken, headerReceipt} {
		if value := headers.Get(header); value != "" {
			fmt.Fprintf(out, "%s: %s\n", header, value)
		}
	}
}

func apiError(status int, body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return fmt.Errorf("API request failed with HTTP %d", status)
	}
	if len(body) > 4096 {
		body = append(body[:4096], []byte("...")...)
	}
	return fmt.Errorf("API request failed with HTTP %d: %s", status, body)
}

func printUsage(out io.Writer) {
	fmt.Fprint(out, `Vectorizer.AI command-line client.

Usage:
  vectorizer [global flags] <command> [command flags]

Global flags:
  --api-id string       API Id. Defaults to VECTORIZER_API_ID.
  --api-secret string   API Secret. Defaults to VECTORIZER_API_SECRET.
  --api-url string      API base URL. Defaults to https://api.vectorizer.ai/api/v1.
  --timeout duration    HTTP timeout. Defaults to 10m.

Commands:
  vectorize INPUT -o OUTPUT [--format svg] [--retention-days N]
  vectorize --url URL -o OUTPUT [--format svg]
  download IMAGE_TOKEN -o OUTPUT [--format svg]
  delete IMAGE_TOKEN
  account
  version

Examples:
  vectorizer vectorize logo.png -o logo.svg
  vectorizer vectorize logo.png -o logo.pdf --format pdf
  vectorizer vectorize --url https://example.com/logo.png -o logo.svg
  vectorizer vectorize logo.png -o logo.svg --retention-days 7
  vectorizer download IMAGE_TOKEN -o logo.pdf --format pdf
  vectorizer account

Advanced API form fields can be passed literally:
  vectorizer vectorize logo.png -o logo.svg --param processing.max_colors=16

`)
}
