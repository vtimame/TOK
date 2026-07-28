# Release Distribution Trust

TOK release distribution remains conservative for alpha. The release workflow
may prepare assets, checksums and signatures, but publishing a tag or GitHub
Release still requires explicit human approval.

## SHA256SUMS Signature

Decision: sign `SHA256SUMS` with Sigstore keyless signing through GitHub Actions
OIDC.

The release workflow installs cosign, signs the checksum file with:

```bash
cosign sign-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json --yes
```

and publishes `SHA256SUMS.sigstore.json` next to `SHA256SUMS`.

Consumers verify with the published bundle and GitHub Actions identity:

```bash
TAG=v0.2.1
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity "https://github.com/vtimame/TOK/.github/workflows/release.yml@refs/tags/${TAG}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Then verify the downloaded archive against `SHA256SUMS`.

## SBOM And Provenance Plan

Next release-hardening step:

- generate a CycloneDX or SPDX SBOM for each release archive;
- publish SBOM files as release assets;
- add GitHub artifact attestations for each `.tar.gz` with `actions/attest@v4`;
- grant the workflow `id-token: write`, `contents: read` and
  `attestations: write` for attestation jobs;
- document verification commands once the attestation assets are present.

This is planned but not required for the current alpha patch. Do not claim SBOM
or provenance coverage in release notes until the workflow produces and verifies
those artifacts.

## Windows Path

Decision: Windows prebuilt binaries are deferred for the current alpha. Windows
users should build from source with a supported Go/Node/pnpm toolchain until the
project adds Windows CI coverage and an install/update path.

Before enabling Windows assets:

- add a `windows-amd64` build matrix entry;
- smoke-test `tok version`, `tok init` and a minimal task/run workflow on
  Windows runners;
- document PowerShell install and checksum/signature verification commands;
- decide whether web UI embedding and CGO settings require a different build
  path.
