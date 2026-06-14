# syntax=docker/dockerfile:1

# Obscura headless-browser image for the Akamai-interstitial fallback.
#
# It downloads the prebuilt, STEALTH-enabled release binary from upstream rather
# than compiling from source. Two reasons:
#   1. Build speed: the from-source build compiles Rust + V8 + BoringSSL
#      (~20 min). Downloading the release tarball is a few seconds.
#   2. Stealth: the published `h4ckf0r0day/obscura` Docker Hub image is built
#      WITHOUT `--features stealth` (no TLS impersonation) and cannot clear
#      Akamai. The GitHub *release* tarball is built with stealth, so `--stealth`
#      actually impersonates a browser TLS fingerprint.
ARG OBSCURA_VERSION=v0.1.8

FROM debian:bookworm-slim AS fetch
ARG OBSCURA_VERSION
# TARGETARCH is provided by BuildKit (amd64 / arm64).
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
    # Fail the build early if the binary can't run (e.g. glibc/snapshot mismatch).
    # `--version` does not init V8, so it is safe under QEMU cross-build.
    /out/obscura --version

# distroless/cc: glibc + libgcc + CA certs only — matches obscura's own runtime.
FROM gcr.io/distroless/cc-debian12
COPY --from=fetch /out/obscura /usr/local/bin/obscura
COPY --from=fetch /out/obscura-worker /usr/local/bin/obscura-worker
EXPOSE 9222
ENTRYPOINT ["/usr/local/bin/obscura"]
CMD ["serve", "--port", "9222", "--host", "0.0.0.0"]
