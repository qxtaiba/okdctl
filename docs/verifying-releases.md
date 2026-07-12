# Verifying a release

Every tagged release ships:

- `okdctl_<version>_linux_<arch>.tar.gz` — binary archive (amd64 + arm64)
- `SHA256SUMS`, `SHA256SUMS.sig`, `SHA256SUMS.pem` — sigstore keyless signature
- `okdctl_<version>_linux_<arch>.sbom.json` — CycloneDX SBOM (binary archive)
- `okdctl_<version>_linux_<arch>.{deb,rpm}.sbom.json` — CycloneDX SBOMs (apt/rpm packages)
- `okdctl.intoto.jsonl` — SLSA build provenance

## Signature

Verify without managing any keys:

```sh
cosign verify-blob \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/qxtaiba/okdctl/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
```

This proves the checksums file came from a GitHub Actions workflow in this
repository — no maintainer private keys, no trust in the release page
markup, no trust in any CDN between you and GitHub.

## Attestation

Each release also carries a GitHub artifact attestation (SLSA build
provenance recorded in the repository's attestations log):

```sh
gh attestation verify <file> --repo qxtaiba/okdctl
```

Checks the attestation against GitHub's Sigstore instance and confirms the
artifact was produced by a workflow in this repository. Requires the
[GitHub CLI](https://cli.github.com/) (`gh`).

## Reproducible builds

Binaries build with `-trimpath` and deterministic ldflags: `make build`
from the tagged commit produces a byte-identical binary. `sha256sum
bin/okdctl` should match `SHA256SUMS`.

## Packages and building from source

`.deb`/`.rpm` packages are on the [releases
page](https://github.com/qxtaiba/okdctl/releases) for apt/dnf users.

From source:

```sh
git clone https://github.com/qxtaiba/okdctl
cd okdctl
make build
sudo install -m 0755 bin/okdctl /usr/local/bin/
```
