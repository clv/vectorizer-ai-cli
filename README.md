# Vectorizer.AI CLI

Official command-line client for the [Vectorizer.AI](https://vectorizer.ai) image vectorization API.

The CLI is distributed as a single native executable named `vectorizer` for Windows, macOS, and Linux. It has no runtime dependency on Python, Node.js, Java, .NET, PHP, Ruby, or the generated SDK packages.

## Downloads

Use the download table in the [latest GitHub release](https://github.com/clv/vectorizer-ai-cli/releases/latest) to pick the build for your platform:

| Platform | Artifact |
| --- | --- |
| Windows x64 | `vectorizer_VERSION_windows_amd64.zip` |
| Windows ARM64 | `vectorizer_VERSION_windows_arm64.zip` |
| macOS Apple Silicon | `vectorizer_VERSION_darwin_arm64.tar.gz` |
| macOS Intel | `vectorizer_VERSION_darwin_amd64.tar.gz` |
| Linux x64 | `.tar.gz`, `.deb`, `.rpm`, or `.apk` |
| Linux ARM64 | `.tar.gz`, `.deb`, `.rpm`, or `.apk` |

GitHub may collapse the raw asset list behind "Show more" when many platform packages are attached, so the release notes keep direct per-platform links at the top.

## Authentication

Set your API credentials in the environment:

```sh
export VECTORIZER_API_ID="your-api-id"
export VECTORIZER_API_SECRET="your-api-secret"
```

On Windows PowerShell:

```powershell
$env:VECTORIZER_API_ID = "your-api-id"
$env:VECTORIZER_API_SECRET = "your-api-secret"
```

You can also pass credentials before the command:

```sh
vectorizer --api-id your-api-id --api-secret your-api-secret account
```

## Usage

Vectorize a local image:

```sh
vectorizer vectorize logo.png -o logo.svg
```

Generate another output format:

```sh
vectorizer vectorize logo.png -o logo.pdf --format pdf
```

Vectorize an image by URL:

```sh
vectorizer vectorize --url https://example.com/logo.png -o logo.svg
```

Retain an image token for later downloads:

```sh
vectorizer vectorize logo.png -o logo.svg --retention-days 7
```

If the API returns `X-Image-Token` or `X-Receipt`, the CLI prints those headers to stderr.

Download a retained result:

```sh
vectorizer download IMAGE_TOKEN -o logo.pdf --format pdf
```

Delete a retained image:

```sh
vectorizer delete IMAGE_TOKEN
```

Check account status:

```sh
vectorizer account
```

Pass advanced API form fields literally with `--param key=value`:

```sh
vectorizer vectorize logo.png -o logo.svg --param processing.max_colors=16
```

## Release Artifacts

GitHub Releases produce:

- Windows x64 and arm64 zip archives
- macOS x64 and arm64 tarballs
- Linux x64 and arm64 tarballs
- Linux `.deb`, `.rpm`, and `.apk` packages
- SHA-256 checksums

Linux builds are made with `CGO_ENABLED=0` so the binary is static and works across ordinary distributions without libc package coupling.

## Development

```sh
go test ./...
go run ./cmd/vectorizer version
```

## License

Apache-2.0
