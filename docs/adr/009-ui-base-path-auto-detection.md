# ADR-009: UI Base-Path Auto-Detection

* **Status**: Implemented
* **Date**: 2026-05-12

## Context

Jaeger UI can be served at a URL prefix (e.g. `/jaeger/`) instead of the root `/`.
Prior to this change, that prefix had to be configured in the Jaeger backend via
`extensions.jaeger_query.base_path`. The backend did two things at startup:

1. **Rewrite `index.html`** – replace the literal string `<base href="/"` with
   `<base href="/jaeger/"` using a regular-expression substitution
   (`static_handler.go:basePathPattern`).
2. **Register all HTTP routes with the prefix** – API routes such as
   `/api/traces` were registered as `/jaeger/api/traces`.

The UI read the `<base>` element's `href` at runtime (`site-prefix.ts`) and
used it as the authoritative path prefix for both asset loading and API calls
(`prefix-url.ts`).  Because the build uses relative paths (`"homepage": "."` in
`package.json`, `base: './'` in Vite config), static assets already loaded
correctly regardless of the prefix.  The `<base>` tag was the only piece of
information the UI actually needed from the backend.

### Use Cases

#### UC-1: Single prefix, same internal and external

The common case: browser and internal cluster traffic both reach Jaeger at
the same path (e.g. `/jaeger/`).  `extensions.jaeger_query.base_path: /jaeger` was set once and
everything worked.

#### UC-2: Single pod, multiple external prefixes

A single Jaeger deployment must be reachable under **different URL prefixes from
different domains or ingress rules** (e.g. `https://team-a.example.com/jaeger/`
and `https://team-b.example.com/traces/`).  Because `index.html` was baked with
one static `<base href>`, only the one matching `base_path` worked; the
other showed a blank page or 404s on static assets.  This was the core limitation
this ADR addressed.

#### UC-3: Proxy rewrites the external prefix (issue #5157 / PR #5219)

The browser accesses Jaeger at an **external prefix** (`/foo/bar/`) that is
different from the **internal prefix** Jaeger listens on (`/baz/`).  A reverse
proxy (e.g. Istio virtual service, NGINX) rewrites the URL on the way in:

```
Browser → /foo/bar/search         (external / UI base path)
  Proxy rewrites →  /baz/search   (internal / API base path)
    Jaeger sees: GET /baz/search
```

The attempted fix in PR #5219 was a new `--query.ui-base-path` flag that would
inject a different value into `<base href>` than the one used for API route
registration.  That PR was never merged because it still had static-asset loading
bugs caused by the same fundamental issue: the correct value for `<base href>` is
the *external* URL, which Jaeger itself never sees (the proxy has already
rewritten it).  The backend could not reliably inject the right value because it
did not know the external prefix.

### Why `<base href>` Is Required

The built Jaeger UI references all static assets with **relative URLs**
(e.g. `<script src="./assets/index-abc123.js">`).  This is intentional: relative
paths let the same build artifact work at any URL prefix without a rebuild or
serve-time rewriting of every asset tag.

The problem is that "relative" means *relative to the current document URL*, and
the document URL changes with every SPA deep-link:

| Browser navigates to | Resolves `./assets/index.js` as |
|---|---|
| `/jaeger/` | `/jaeger/assets/index.js` ✓ |
| `/jaeger/trace/abc123` | `/jaeger/trace/assets/index.js` ✗ |

Without an anchor, any bookmarked or shared deep-link causes 404s on every static
asset.  The HTML `<base href="/jaeger/">` element is the browser-native mechanism
to fix the base URL for all relative references to a stable mount point,
regardless of the current page path.

Switching to absolute asset paths (e.g. `/jaeger/assets/index.js`) would avoid
the need for `<base href>`, but would require the prefix to be known at build
time or injected into every `<link>`/`<script>` tag at serve time — the same
backend-dependency problem, just spread across many tags instead of one.

### Why the `<base>` Tag Must Be Set Before Asset Tags

The `<link rel="stylesheet">` and `<script type="module">` tags in `index.html`
are processed by the browser's **HTML preload scanner** before any JavaScript
executes.  A `<script>` that runs after those tags cannot redirect the requests
that are already in flight.

The only exception is a `<script>` that appears *earlier in document order* than
the asset tags.  Such a script executes synchronously and can write a `<base>`
element into the document before the preload scanner encounters any relative URL.
Setting `window.__webpack_public_path__` (or Vite's equivalent) from JavaScript
only controls dynamically-imported chunks loaded later; it cannot fix the initial
`<link>` and `<script>` tags.

This is why the inline script must be placed **before all asset tags** in `index.html`.

### Key Insight: the Browser Knows the External Prefix

When a browser requests `https://example.com/jaeger/search` and receives an
`index.html` whose `<base href="/">` points to the root, the browser resolves
relative asset URLs against `https://example.com/`, breaking static file loading.

An inline `<script>` that runs *synchronously before any `<link>` or `<script
type="module">` tags* can read `window.location.pathname` — which is the
*external* URL as seen by the browser — and write the correct `<base href>` into
the document before the browser fetches any assets.  This works regardless of
what the proxy did on the way in.

This also solves UC-3: the browser at `/foo/bar/search` computes
the prefix `/foo/bar/` entirely from its own URL, with no input from Jaeger.

## Decision

Remove the backend's responsibility for injecting the base path into the
`<base>` element, and instead make the UI self-detect its own prefix at
page-load time using an inline script.

### Mechanism

Replace the static `<base href="/" …>` in `index.html` with a `<base>` element
whose `href` is set by an inline script that executes before any other resource
is fetched.

`site-prefix.ts` already reads `document.querySelector('base').href`, so the
rest of the UI stack (`prefix-url.ts`, API calls, React Router `basename`)
continues to work without changes.

### API Calls Under a Proxy Prefix Rewrite (UC-3)

After the inline script runs, the UI builds API URLs by prepending the detected
prefix, e.g. `prefixUrl('/api/traces')` → `/foo/bar/api/traces`.  The browser
sends `GET /foo/bar/api/traces` to the proxy, which rewrites to `GET
/baz/api/traces` before forwarding to Jaeger.  Jaeger's route for `/baz/api/traces`
(registered via `extensions.jaeger_query.base_path: /baz`) matches correctly.

This means UC-3 works without any new backend flag:

| Layer | Configuration |
|---|---|
| Browser | accesses `/foo/bar/` — no config needed |
| Proxy | rewrites `/foo/bar/` → `/baz/` |
| Jaeger backend | `extensions.jaeger_query.base_path: /baz` |

The previously proposed `--query.ui-base-path` flag is **not needed**.

### Backend Changes

1. **Remove the base-path injection** from `static_handler.go`
   (`loadAndEnrichIndexHTML`).  The `basePathPattern` regexp and the replacement
   logic are deleted.

2. **Keep `extensions.jaeger_query.base_path` for API route registration only.**
   This setting still controls at which prefix the backend registers HTTP routes
   (e.g. `/baz/api/traces`).  Operators who use a non-root prefix must continue
   to set it so that API calls land on the correct handler.  For deployments
   where the ingress strips the prefix before forwarding, it is not needed at all.

### Handling Deep-Link Requests

The backend must still serve `index.html` for all SPA routes under the prefix so
that direct navigation (e.g. bookmarking `/jaeger/trace/abc123`) works.
This requirement is unchanged; `RegisterRoutes` already does this via a catch-all
handler.

### Development Mode

The Vite dev-server plugin (`vite.config.mts:jaegerUiConfigPlugin`) simulates
backend behavior for local development.  Because the inline script reads
`window.location` directly, it works in the browser-based dev server without any
changes.  The `jaegerUiConfigPlugin` no longer injects a `<base>` tag.

## Alternatives Considered

### A. Keep Current Backend Injection (Status Quo)

**Pros:** Works today; no UI changes needed.

**Cons:** UC-2 (multiple prefixes from one pod) is impossible; UC-3 (proxy
rewrite with different external/internal paths) is impossible because Jaeger
never sees the external prefix; operator burden; silent misconfig failures.

### B. Add `--query.ui-base-path` Flag (PR #5219 approach)

A new flag injects a different value into `<base href>` than the API route prefix.

**Pros:** Separates concerns; no UI changes needed.

**Cons:** Jaeger still cannot know the *external* prefix when a proxy rewrites
URLs — this was the root bug in PR #5219 (assets loaded from the wrong path).
UC-2 (multiple external prefixes from one pod) remains unsupported.
Adds a new flag that operators must keep in sync with both the proxy and
`extensions.jaeger_query.base_path`.

### C. Inline Script with Known Sub-Paths (Proposed)

**Pros:** Zero backend config for UI delivery; a single pod serves under any
prefix; works behind URL-rewriting proxies; derived from the actual browser URL,
so always correct.

**Cons:** Requires maintaining a list of known SPA top-level route segments in
the inline script (mitigated by a unit test).

### D. Read Prefix from `window.location` at Root Entry Only

Always redirect to `/search` and compute the prefix from there.

**Pros:** No list of sub-paths needed.

**Cons:** Breaks direct deep-link navigation (bookmarks, shared trace URLs); poor
UX; not viable for a debugging tool.

### E. Backend Probe Before Render

Strip path components one at a time and probe `/api/services` asynchronously
until a 200 is received.

**Pros:** No static list.

**Cons:** Requires an async network round-trip before any UI renders; complex
error handling; still broken if API is at a different prefix than the UI.

## Consequences

### Positive

* UC-2 solved: a single Jaeger pod can serve its UI under any number of external
  prefixes simultaneously.
* UC-3 solved: proxy-rewrite deployments work without any new Jaeger flag.
* Eliminates the silent misconfiguration failure mode where `base_path` and
  ingress disagree.
* Reduces operator burden: ingress-level prefix stripping requires no additional
  backend flag.
* Backwards-compatible: existing deployments that set `extensions.jaeger_query.base_path`
  continue to work because API routes are still registered at the configured prefix.
* The previously proposed `--query.ui-base-path` flag is no longer needed.

### Negative

* The inline script must enumerate known top-level SPA route segments.  New
  routes must be added to both the router configuration and this list.
* The inline script is imperative JavaScript in `index.html`, which slightly
  complicates the HTML template.
* `extensions.jaeger_query.base_path` becomes partially redundant (needed only
  for API route registration, not UI delivery), which may confuse operators.
  Clear documentation and a future deprecation path are required.

## Test Plan

### Unit tests (automated, in CI)

#### jaeger-ui: inline script

`packages/jaeger-ui/index.test.ts` — exercises the inline script from `index.html`
by extracting and evaluating it against a mock document object:

| Scenario | Input pathname | Expected prefix |
|---|---|---|
| Root, bare slash | `/` | `/` |
| Root, each known sub-path at root | `/search`, `/trace/abc`, `/dependencies`, … | `/` |
| Prefixed, bare prefix | `/jaeger/` | `/jaeger/` |
| Prefixed, each known sub-path | `/jaeger/search`, `/jaeger/trace/abc`, … | `/jaeger/` |
| Deep prefix | `/a/b/c/search` | `/a/b/c/` |
| Unknown path (no sub-path match) | `/jaeger/unknown-page` | `/jaeger/` |

#### jaeger (backend): `static_handler_test.go`

`TestRegisterStaticHandler` — verifies that for all base-path configurations
(`""`, `"/"`, `"/jaeger"`, `"/metrics"`) the served `index.html` contains the
inline script marker (`data-inject-target="BASE_URL"`) and that static assets are
served at the correct route prefix.  The test no longer asserts a specific
`<base href>` value, because the backend no longer writes one.

### Integration / end-to-end tests (manual or in the reverse-proxy example)

The existing `examples/reverse-proxy/` docker-compose should be used to validate
all three use cases:

#### UC-1: Direct access, `base_path` matches ingress

Setup (see `examples/reverse-proxy/httpd.conf`):
- Jaeger: `extensions.jaeger_query.base_path: /jaeger/prefix` (internal routes at `/jaeger/prefix/…`)
- Apache httpd: `ProxyPass /jaeger/prefix http://jaeger:16686/jaeger/prefix` — forwards path unchanged, no rewriting
- Browser URL: `http://localhost:18080/jaeger/prefix/`

Checks:
1. `GET /jaeger/prefix/` → `index.html` loads, inline script present, no static `<base href="/jaeger/prefix/">` injected by backend.
2. Static assets (`/jaeger/prefix/static/index-*.js`) return 200.
3. API `GET /jaeger/prefix/api/services` → 200.
4. Deep-link: `GET /jaeger/prefix/trace/<id>` → 200, serves `index.html` with inline script.

**Result: PASS** ✅

#### UC-2: Same pod, two external prefixes

Setup (see `examples/reverse-proxy/httpd-uc2.conf`):
- Jaeger: no `base_path` configured (serves at root `/`)
- Apache httpd rule A (more specific): `ProxyPass /alt/ http://jaeger:16686/` — strips `/alt`
- Apache httpd rule B: `ProxyPass / http://jaeger:16686/` — pass-through
- Browser URLs: `http://localhost:18081/` and `http://localhost:18081/alt/`

Checks:
1. `GET /` → `index.html` loads, inline script present; script detects prefix `/`.
2. `GET /alt/` → `index.html` loads, inline script present; script detects prefix `/alt/`.
3. Static assets load under both prefixes (`/static/index-*.js` and `/alt/static/index-*.js`).
4. API `GET /api/services` and `GET /alt/api/services` → 200.

**Result: PASS** ✅

#### UC-3: Proxy rewrites external prefix to a different internal prefix

Setup (see `examples/reverse-proxy/httpd-uc3.conf`):
- Jaeger: `extensions.jaeger_query.base_path: /internal`
- Apache httpd: `ProxyPass /external/ http://jaeger:16686/internal/` — rewrites external to internal
- Browser URL: `http://localhost:18082/external/`

Checks:
1. `GET /external/` → `index.html` loads; inline script detects prefix `/external/` from `window.location.pathname`.
2. Static assets at `/external/static/index-*.js` → proxy rewrites to `/internal/static/…` → 200.
3. API `GET /external/api/services` → proxy rewrites to `/internal/api/services` → 200.
4. Deep-link: `GET /external/trace/<dummy-id>` → 200, serves `index.html` with inline script.

**Result: PASS** ✅

### Regression checks

- `base_path` omitted (root deployment): UI loads at `/`, all routes work.
- `extensions.jaeger_query.base_path: /jaeger` with no proxy (current common case): UI loads at `/jaeger/`, all routes work — identical behaviour to before this change.
- Hot-reload of `jaeger-ui.config.json` still works (backend still rewrites the config placeholder in `index.html`; only the `<base>` injection was removed).

## Future Improvements

### Proxy-Hint via `X-Forwarded-Prefix` Header

The pathname-based detection has one known ambiguity: a URL like `/a/search/`
cannot be distinguished from a Jaeger instance mounted at `/a/search/` vs. one
mounted at `/a/` with the `search` sub-path.  Current heuristics resolve most
real-world cases, but a more reliable mechanism is possible.

Many reverse proxies (Traefik, nginx, etc.) can emit an `X-Forwarded-Prefix`
header carrying the external prefix they stripped before forwarding
(e.g. `X-Forwarded-Prefix: /external/`).  The browser cannot read response
headers directly, but Jaeger's backend already injects dynamic values into
`index.html` (config, version, storage capabilities).  It could read this header
from the incoming request and embed it as a `<meta>` tag or inline variable
before the detection script runs, eliminating the sub-path heuristics entirely.

However this approach has significant drawbacks that prevented it from being
adopted here:

* **Injection risk.** `X-Forwarded-Prefix` is a request header — any client can
  send it directly, bypassing the proxy.  Embedding an unsanitised value into
  HTML would be a stored-XSS vector.  Strict sanitisation (allowlist `[a-zA-Z0-9/_-]`)
  is necessary but not sufficient; a trusted-proxy allowlist (by IP or network)
  is also required to prevent spoofing — the same operational burden as
  `X-Forwarded-For`.

* **Per-request HTML generation.** The current backend caches `index.html` at
  startup.  Reading a per-request header would require generating a fresh
  response body on every page load, adding latency and complexity.

* **Not universally supported.** Apache httpd does not emit `X-Forwarded-Prefix`
  automatically; it requires an explicit `RequestHeader set X-Forwarded-Prefix`
  directive per proxy rule.  Operators who forget it get a silent regression to
  broken behavior.

* **Operational complexity.** Every proxy rule must be updated, trusted-proxy
  lists must be maintained, and the security posture must be audited — trading a
  maintained list of SPA route segments for a new class of infrastructure and
  security concerns.

The inline-script approach has none of these requirements and zero security
surface.  The `X-Forwarded-Prefix` path remains a theoretical option for
deployments that genuinely cannot tolerate the sub-path list, but the costs
outweigh the benefit for the common case.

## References

- `jaeger-ui/packages/jaeger-ui/index.html` – inline base-path detection script
- `jaeger-ui/packages/jaeger-ui/src/site-prefix.ts` – runtime prefix detection
- `jaeger-ui/packages/jaeger-ui/src/utils/prefix-url.ts` – prefix application to URLs
- `cmd/jaeger/internal/extension/jaegerquery/internal/static_handler.go` – API route registration and index.html serving
- [jaeger-ui issue #42](https://github.com/jaegertracing/jaeger-ui/issues/42) – original motivation for `<base>` + relative paths
- [jaeger issue #5157](https://github.com/jaegertracing/jaeger/issues/5157) – feature request: support external URL prefix (proxy rewrite case)
- [jaeger PR #5219](https://github.com/jaegertracing/jaeger/pull/5219) – attempted `--query.ui-base-path` implementation (not merged)
