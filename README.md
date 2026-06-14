# SaeidRp/obscura

A thin Docker image that packages the **stealth-enabled** [obscura](https://github.com/h4ckf0r0day/obscura)
headless browser, published to GHCR for use as the **Akamai-interstitial fallback**
in the SaeidRp backend (Magnific/Freepik page fetches).

## Why this exists

- **Stealth is required.** Upstream's published `h4ckf0r0day/obscura` Docker Hub
  image is built **without** `--features stealth`, so `--stealth` only does
  tracker blocking — no TLS-fingerprint impersonation, and it cannot clear
  Akamai. The upstream **release tarballs**, however, are built *with* stealth.
- **Per-request proxy.** obscura only accepts a per-request proxy through the
  `fetch` CLI (`--proxy`); `serve`/CDP can pin just one proxy at startup. The
  backend rotates proxies per request, so this image ships a small **HTTP
  sidecar** that turns each request into a one-shot `obscura fetch` invocation.

So the `Dockerfile` downloads the upstream stealth release binary for the target
architecture, builds the Go sidecar, and drops both into a `distroless/cc`
runtime that runs the sidecar.

## Sidecar API

```
POST /fetch
{
  "url":        "https://…",                       // required
  "proxy":      "http://user:pass@ip:port",        // per-request proxy
  "user_agent": "Mozilla/5.0 …",                   // optional
  "cookies":    [ { "name": "…", "value": "…", "domain": "…", "path": "/" } ],
  "timezone":   "Europe/Amsterdam",                // optional, match proxy region
  "geolocation":"52.36,4.90",                      // optional
  "timeout":    60                                  // optional, obscura nav seconds
}
→ 200 { "status": 200, "html": "<…rendered…>", "cookies": [ … ] }
→ 502 { "error": "obscura fetch failed: …" }

GET /health → 200 ok
```

Each request runs in an isolated process with its own `--storage-dir`, so cookies
and concurrency are isolated. `fetch` exposes User-Agent (not arbitrary headers);
obscura's stealth profile supplies a coherent browser header set.

## Image

```
ghcr.io/saeidrp/obscura:<upstream-version>   # e.g. v0.1.8 (lowercase — GHCR requires it)
ghcr.io/saeidrp/obscura:latest
```

Multi-arch: `linux/amd64` + `linux/arm64`.

## Build / run locally

```bash
docker build --build-arg OBSCURA_VERSION=v0.1.8 -t ghcr.io/saeidrp/obscura:v0.1.8 .
docker run --rm -p 9222:9222 ghcr.io/saeidrp/obscura:v0.1.8
curl -s -XPOST localhost:9222/fetch -H 'content-type: application/json' \
  -d '{"url":"https://example.com","proxy":"http://user:pass@ip:port"}'
```

## Caveats

- Pin the version in the backend's `OBSCURA_VERSION`; don't blindly track `latest`
  in production.
- obscura's own docs say stealth does **not** officially handle Akamai *active*
  challenges. It clears Magnific's *soft* interstitial today; monitor for breakage.
