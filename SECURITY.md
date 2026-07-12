# Security policy

## Reporting a vulnerability

Report vulnerabilities privately via [GitHub Security
Advisories](https://github.com/qxtaiba/okdctl/security/advisories/new)
on this repository. Do not open public issues for vulnerabilities.

You should expect an acknowledgement within 72 hours. Coordinated disclosure
timelines are negotiated case by case; the default is 90 days from acknowledgement
to public disclosure, shorter if a fix ships sooner.

## Supported versions

Pre-1.0, only the latest minor release is supported. Once 1.0 ships, this
policy will be updated to cover the two most recent minors.

## Release integrity

Every tagged release is signed with [sigstore](https://www.sigstore.dev/)
(keyless, via GitHub OIDC) and ships with a CycloneDX SBOM and SLSA build
provenance. Verification is documented in
[docs/verifying-releases.md](docs/verifying-releases.md).

If `cosign verify-blob` fails against a release you downloaded, treat the
artifact as compromised and report it via the advisory process above.

## Deploy-time considerations

The ignition pull-secret exposure window during bootstrap and the SSH/SCP
host-key trust-on-first-use behavior are documented in
[docs/security.md](docs/security.md).
