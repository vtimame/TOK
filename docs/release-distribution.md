# Release Distribution Trust

TOK release distribution remains conservative for alpha. The release workflow
may prepare assets, checksums, signatures and attestations, but publishing a tag
or GitHub Release still requires explicit human approval.

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
TAG=v0.3.0
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity "https://github.com/vtimame/TOK/.github/workflows/release.yml@refs/tags/${TAG}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Then verify the downloaded archive against `SHA256SUMS`.

## Artifact Provenance Attestations

Decision: generate GitHub Artifact Attestations for each release `.tar.gz`
archive produced by the release workflow after this hardening change.

The release workflow attests each archive after packaging it and before
publishing the release. Attestations are stored by GitHub and linked to the
repository rather than uploaded as separate release assets.

After downloading an archive, verify its provenance with GitHub CLI:

```bash
gh attestation verify tok_linux_amd64.tar.gz \
  --repo vtimame/TOK \
  --signer-workflow vtimame/TOK/.github/workflows/release.yml \
  --source-ref refs/tags/<release-tag>
```

Run the same command for each downloaded `.tar.gz` archive. GitHub CLI verifies
build provenance by default. Keep the existing `SHA256SUMS` and
`SHA256SUMS.sigstore.json` verification: attestations prove where the archive
was built, while checksums and the Sigstore bundle protect the published
checksum file and archive digest.

## SBOM Plan

Next release-hardening step:

- generate a CycloneDX or SPDX SBOM for each release archive;
- publish SBOM files as release assets;
- optionally add SBOM attestations after the SBOM files are generated;
- document SBOM verification commands once SBOM artifacts are present.

This is planned but not required for the current alpha patch. Do not claim SBOM
coverage in release notes until the workflow produces and verifies those
artifacts.

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
