# syntax=docker/dockerfile:1

# Obscura image for the Akamai-interstitial fallback.
#
# Ships the prebuilt STEALTH-enabled obscura release binary plus a small HTTP
# sidecar. The sidecar turns `POST /fetch {url, proxy, ...}` into a one-shot
# `obscura fetch --proxy <proxy> --stealth` invocation — the only way obscura
# supports a PER-REQUEST proxy (serve/CDP can only pin one proxy at startup).
# See README for the rationale.
ARG OBSCURA_VERSION=v0.1.8

# --- download the upstream stealth release binary --------------------------
FROM debian:bookworm-slim AS obscura
ARG OBSCURA_VERSION
ARG TARGETARCH
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN set -eux; \
    case "$TARGETARCH" in \
        amd64) asset="obscura-x86_64-linux.tar.gz" ;; \
        arm64) asset="obscura-aarch64-linux.tar.gz" ;; \
        *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/obscura.tar.gz \
        "https://github.com/h4ckf0r0day/obscura/releases/download/${OBSCURA_VERSION}/${asset}"; \
    mkdir -p /out; \
    tar -xzf /tmp/obscura.tar.gz -C /out obscura obscura-worker; \
    chmod +x /out/obscura /out/obscura-worker; \
    # `--version` does not init V8, so it is safe under QEMU cross-build.
    /out/obscura --version

# --- build the Go sidecar (cross-compiled, static) -------------------------
FROM --platform=$BUILDPLATFORM golang:1.22-bookworm AS sidecar
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY sidecar ./sidecar
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/obscura-sidecar ./sidecar

# --- runtime ---------------------------------------------------------------
# distroless/cc: glibc + libgcc + CA certs (obscura needs glibc; the sidecar is
# static). Default user is root so /tmp storage dirs are writable.
FROM gcr.io/distroless/cc-debian12
COPY --from=obscura /out/obscura /usr/local/bin/obscura
COPY --from=obscura /out/obscura-worker /usr/local/bin/obscura-worker
COPY --from=sidecar /out/obscura-sidecar /usr/local/bin/obscura-sidecar
EXPOSE 9222
ENTRYPOINT ["/usr/local/bin/obscura-sidecar"]
