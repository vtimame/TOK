# Install TOK

TOK is distributed as a single `tok` binary. The release binary embeds the web
UI, so the recommended path is to download a prebuilt asset from GitHub
Releases.

## Requirements

For release binaries:

- a supported operating system and architecture;
- a local git repository to track as a TOK project.

For source builds:

- Go 1.26.5 or newer;
- Node.js 24 or newer;
- pnpm 10 or newer.

## Release Assets

The first release workflow builds assets for:

- Linux x86_64: `tok_linux_amd64.tar.gz`
- macOS Apple Silicon: `tok_darwin_arm64.tar.gz`
- macOS Intel: `tok_darwin_amd64.tar.gz`
- Checksums: `SHA256SUMS`
- Checksum signature bundle: `SHA256SUMS.sigstore.json`

Windows binaries are not part of the first alpha release.

Download `SHA256SUMS` next to the asset you want to install and verify the
archive before extracting it. For releases that include
`SHA256SUMS.sigstore.json`, verify the checksum file before checking the
archive:

```bash
TAG=v0.2.1
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity "https://github.com/vtimame/TOK/.github/workflows/release.yml@refs/tags/${TAG}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Linux x86_64

```bash
curl -L -o tok_linux_amd64.tar.gz \
  https://github.com/vtimame/TOK/releases/latest/download/tok_linux_amd64.tar.gz
curl -L -o SHA256SUMS \
  https://github.com/vtimame/TOK/releases/latest/download/SHA256SUMS
grep ' tok_linux_amd64.tar.gz$' SHA256SUMS | sha256sum -c -
tar -xzf tok_linux_amd64.tar.gz
chmod +x tok
sudo mv tok /usr/local/bin/tok
```

## macOS Apple Silicon

```bash
curl -L -o tok_darwin_arm64.tar.gz \
  https://github.com/vtimame/TOK/releases/latest/download/tok_darwin_arm64.tar.gz
curl -L -o SHA256SUMS \
  https://github.com/vtimame/TOK/releases/latest/download/SHA256SUMS
grep ' tok_darwin_arm64.tar.gz$' SHA256SUMS | shasum -a 256 -c -
tar -xzf tok_darwin_arm64.tar.gz
chmod +x tok
sudo mv tok /usr/local/bin/tok
```

## macOS Intel

```bash
curl -L -o tok_darwin_amd64.tar.gz \
  https://github.com/vtimame/TOK/releases/latest/download/tok_darwin_amd64.tar.gz
curl -L -o SHA256SUMS \
  https://github.com/vtimame/TOK/releases/latest/download/SHA256SUMS
grep ' tok_darwin_amd64.tar.gz$' SHA256SUMS | shasum -a 256 -c -
tar -xzf tok_darwin_amd64.tar.gz
chmod +x tok
sudo mv tok /usr/local/bin/tok
```

Verify the installed binary:

```bash
tok version
```

## Build From Source

For the first alpha, prefer `make build` over `go install`: the release binary
embeds the built web UI, and `make build` runs that web build step.

```bash
git clone https://github.com/vtimame/TOK.git tok
cd tok
make build
```

This writes the binary to:

```bash
./bin/tok
```

For convenience while trying the project locally:

```bash
alias tok="$PWD/bin/tok"
```

## Initialize Storage

```bash
tok init
tok user set-name "Your Name"
```

TOK stores local state under the TOK data directory. Set `TOK_DATA_DIR` if you
want test data separate from your normal local workspace:

```bash
export TOK_DATA_DIR="$PWD/.tok-data"
tok init
```

## Troubleshooting

If `tok` is not found after moving it to `/usr/local/bin`, make sure that
directory is on your `PATH`:

```bash
echo "$PATH"
```

If `make build` fails in `web`, install the expected toolchain and retry:

```bash
node --version
pnpm --version
go version
cd web && pnpm install
cd .. && make build
```

If `tok mcp serve` exits immediately in a terminal, run it through an MCP
client. The command uses stdio and exits when stdin closes.

For worker agents, prefer the smaller tool profile:

```bash
TOK_AGENT_TOKEN="tok_agent_..." tok mcp serve --profile worker
```

Windows prebuilt binaries are deferred for the current alpha. On Windows, build
from source until the project adds Windows CI coverage and a documented
PowerShell install path.
