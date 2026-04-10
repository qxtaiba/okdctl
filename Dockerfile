# syntax=docker/dockerfile:1
#
# This Dockerfile is consumed by goreleaser during release builds — the
# binary is pre-built by goreleaser and COPY'd in. For local iteration,
# use `make build` instead.
#
# We use distroless base to minimize surface area: no shell, no package
# manager, no userland. openshitctl itself is a static binary (CGO_ENABLED=0).
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/qxtaiba/okd-proxmox-cli"
LABEL org.opencontainers.image.description="Deploy OKD clusters on Proxmox with one binary"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY openshitctl /usr/local/bin/openshitctl

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/openshitctl"]
