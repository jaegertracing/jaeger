# Reverse Proxy Example

This example shows how to deploy Jaeger UI behind a reverse proxy that serves it under a URL prefix. Three use cases are demonstrated, covering the most common proxy configurations.

Jaeger UI auto-detects its mount-point prefix from `window.location.pathname` at page-load time (see [ADR-009](../../docs/adr/009-ui-base-path-auto-detection.md)), so it works correctly in all three cases without any browser-side configuration.

## Use Cases

### UC-1: Proxy forwards the prefix unchanged

The most common setup. The external URL prefix and the internal URL prefix are the same. `extensions.jaeger_query.base_path` tells Jaeger which prefix to register API routes under.

```
browser → /jaeger/prefix/... → proxy → http://jaeger:16686/jaeger/prefix/...
```

Jaeger is started with `--set extensions.jaeger_query.base_path=/jaeger/prefix`.

### UC-2: Single pod served under two external prefixes

One Jaeger instance (no `base_path` configured) is reachable at both `/` and `/alt/`. The proxy strips the `/alt/` prefix before forwarding, so Jaeger always receives requests at its root. The UI detects the correct external prefix from the browser URL in each case.

```
browser → /...        → proxy → http://jaeger:16686/...       (pass-through)
browser → /alt/...    → proxy → http://jaeger:16686/...       (strips /alt)
```

### UC-3: Proxy rewrites external prefix to a different internal prefix

The external prefix visible to the browser (`/external/`) differs from the internal prefix Jaeger registers (`/internal/`). This was previously impossible because the backend could only inject the internal prefix it knew about. With auto-detection the UI reads the external prefix from the browser URL and constructs API requests using it, which the proxy then rewrites to the internal path.

```
browser → /external/... → proxy → http://jaeger:16686/internal/...
```

Jaeger is started with `--set extensions.jaeger_query.base_path=/internal`.

## Running the Example

All three use cases are defined in `docker-compose.yml`. Start everything with:

```sh
cd examples/reverse-proxy
JAEGER_IMAGE=cr.jaegertracing.io/jaegertracing/jaeger:latest docker compose up
```

| Use case | URL |
|----------|-----|
| UC-1 | http://localhost:18080/jaeger/prefix/ |
| UC-2 root | http://localhost:18081/ |
| UC-2 /alt/ | http://localhost:18081/alt/ |
| UC-3 | http://localhost:18082/external/ |

Note: accessing without the trailing slash (e.g. `/jaeger/prefix`) returns a 301 redirect to the canonical URL with trailing slash.

## Configuration Files

| File | Purpose |
|------|---------|
| `httpd.conf` | Apache config for UC-1 |
| `httpd-uc2.conf` | Apache config for UC-2 |
| `httpd-uc3.conf` | Apache config for UC-3 |
| `docker-compose.yml` | All three stacks on isolated networks |
