# SaeidRp/obscura

A thin Docker image that packages the **stealth-enabled** [obscura](https://github.com/h4ckf0r0day/obscura)
headless browser, published to GHCR for use as the **Akamai-interstitial fallback**
in the SaeidRp backend (Magnific/Freepik page fetches).

## Why this exists

- **Stealth is required.** Upstream's published `h4ckf0r0day/obscura` Docker Hub
  image is built **without** `--features stealth`, so `--stealth` only does
  tracker blocking — no TLS-fingerprint impersonation, and it cannot clear
  Akamai. The upstream **release tarballs**, however, are built *with* stealth.

So the `Dockerfile` downloads the upstream stealth release binary for the target
architecture and drops it into a `distroless/cc` runtime.

## Image

```
ghcr.io/SaeidRp/obscura:<upstream-version>   # e.g. v0.1.8
ghcr.io/SaeidRp/obscura:latest
```

Multi-arch: `linux/amd64` + `linux/arm64`.

## Build locally

```bash
docker build --build-arg OBSCURA_VERSION=v0.1.8 -t ghcr.io/SaeidRp/obscura:v0.1.8 .
docker run --rm -p 9222:9222 ghcr.io/SaeidRp/obscura:v0.1.8 \
  serve --host 0.0.0.0 --stealth --proxy "http://user:pass@host:port"
```

## Caveats

- Pin the version in the backend's `OBSCURA_VERSION`; don't blindly track `latest`
  in production.
