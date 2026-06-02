package main

import (
	"flag"
	"reflect"
	"testing"
)

func TestParseCommandFlagsAllowsFlagsAfterPositional(t *testing.T) {
	fs := flag.NewFlagSet("vectorize", flag.ContinueOnError)
	output := fs.String("o", "", "")
	format := fs.String("format", "", "")

	rest, err := parseCommandFlags(fs, []string{"logo.png", "-o", "logo.svg", "--format", "pdf"}, map[string]bool{
		"o": true, "format": true,
	})
	if err != nil {
		t.Fatalf("parseCommandFlags returned error: %v", err)
	}
	if *output != "logo.svg" {
		t.Fatalf("output flag = %q, want logo.svg", *output)
	}
	if *format != "pdf" {
		t.Fatalf("format flag = %q, want pdf", *format)
	}
	if !reflect.DeepEqual(rest, []string{"logo.png"}) {
		t.Fatalf("rest = %#v, want logo.png", rest)
	}
}

func TestParseCommandFlagsRespectsDashDash(t *testing.T) {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	output := fs.String("o", "", "")

	rest, err := parseCommandFlags(fs, []string{"-o", "out.svg", "--", "--literal-token"}, map[string]bool{
		"o": true,
	})
	if err != nil {
		t.Fatalf("parseCommandFlags returned error: %v", err)
	}
	if *output != "out.svg" {
		t.Fatalf("output flag = %q, want out.svg", *output)
	}
	if !reflect.DeepEqual(rest, []string{"--literal-token"}) {
		t.Fatalf("rest = %#v, want literal token", rest)
	}
}

func TestParseParams(t *testing.T) {
	fields, err := parseParams(repeatedFlag{"processing.max_colors=16", "custom=value=with=equals"})
	if err != nil {
		t.Fatalf("parseParams returned error: %v", err)
	}
	if fields["processing.max_colors"] != "16" {
		t.Fatalf("processing.max_colors = %q, want 16", fields["processing.max_colors"])
	}
	if fields["custom"] != "value=with=equals" {
		t.Fatalf("custom = %q, want value with equals", fields["custom"])
	}
}

func TestParseParamsRejectsMissingKey(t *testing.T) {
	if _, err := parseParams(repeatedFlag{"=value"}); err == nil {
		t.Fatal("parseParams accepted an empty key")
	}
}

func TestCountNonEmptyTrimsWhitespace(t *testing.T) {
	if got := countNonEmpty("input.png", " ", "", "token"); got != 2 {
		t.Fatalf("countNonEmpty = %d, want 2", got)
	}
}
