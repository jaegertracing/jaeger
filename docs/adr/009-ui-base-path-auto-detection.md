# ADR-009: UI Base-Path Auto-Detection

* **Status**: Proposed
* **Date**: 2026-05-12

## Context

Jaeger UI can be served at a URL prefix (e.g. `/jaeger/`) instead of the root `/`.
Today that prefix must be configured in the Jaeger backend via `--query.base-path`.
The backend then does two things at startup:

1. **Rewrites `index.html`** – replaces the literal string `<base href="/"` with
   `<base href="/jaeger/"` using a regular-expression substitution
   (`static_handler.go:basePathPattern`).
2. **Registers all HTTP routes with the prefix** – API routes such as
   `/api/traces` are registered as `/jaeger/api/traces`.

The UI reads the `<base>` element's `href` at runtime (`site-prefix.ts`) and
uses it as the authoritative path prefix for both asset loading and API calls
(`prefix-url.ts`).  Because the build uses relative paths (`"homepage": "."` in
`package.json`, `base: './'` in Vite config), static assets already load
correctly regardless of the prefix.  The `<base>` tag is the only piece of
information the UI actually needs from the backend.

### Use Cases

#### UC-1: Single prefix, same internal and external

The common case today: browser and internal cluster traffic both reach Jaeger at
the same path (e.g. `/jaeger/`).  `--query.base-path=/jaeger` is set once and
everything works.

#### UC-2: Single pod, multiple external prefixes

A single Jaeger deployment must be reachable under **different URL prefixes from
different domains or ingress rules** (e.g. `https://team-a.example.com/jaeger/`
and `https://team-b.example.com/metrics/`).  Because `index.html` is baked with
one static `<base href>`, only the one matching `--query.base-path` works; the
other shows a blank page or 404s on static assets.  This is the core limitation
this ADR addresses.

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
rewritten it).  The backend cannot reliably inject the right value because it
does not know the external prefix.

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
the need for `<base href>`, but it would require the prefix to be known at build
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

This is why the proposed inline script must be placed **before all asset tags**
in `index.html`.

### Key Insight: the Browser Knows the External Prefix

When a browser requests `https://example.com/jaeger/search` and receives an
`index.html` whose `<base href="/">` points to the root, the browser resolves
relative asset URLs against `https://example.com/`, breaking static file loading.

But if an inline `<script>` runs *synchronously before any `<link>` or `<script
type="module">` tags*, it can read `window.location.pathname` — which is the
*external* URL as seen by the browser — and write the correct `<base href>` into
the document before the browser fetches any assets.  This works regardless of
what the proxy did on the way in.

Critically, this also solves UC-3: the browser at `/foo/bar/search` will compute
the prefix `/foo/bar/` entirely from its own URL, with no input from Jaeger.

## Decision

Remove the backend's responsibility for injecting the base path into the
`<base>` element, and instead make the UI self-detect its own prefix at
page-load time using an inline script.

### Mechanism

Replace the static `<base href="/" …>` in `index.html` with a `<base>` element
whose `href` is set by an inline script that executes before any other resource
is fetched:

```html
<!-- Auto-detect the base path from the current URL.
     index.html is always served for every UI route, so window.location.pathname
     gives us the full prefix (e.g. "/jaeger/trace/abc123") from which we can
     derive the mount point ("/jaeger/"). -->
<script>
  (function() {
    // The backend re-serves index.html for all SPA deep-links under the prefix.
    // Strip any known SPA sub-path to isolate the mount point.
    var knownSubPaths = ['/search', '/trace/', '/monitor', '/dependencies', '/api/'];
    var path = window.location.pathname;
    var prefix = path;
    for (var i = 0; i < knownSubPaths.length; i++) {
      var idx = path.indexOf(knownSubPaths[i]);
      if (idx !== -1) {
        prefix = path.slice(0, idx + 1); // keep trailing slash
        break;
      }
    }
    if (prefix[prefix.length - 1] !== '/') prefix += '/';
    document.currentScript.insertAdjacentHTML(
      'afterend',
      '<base href="' + prefix + '" data-inject-target="BASE_URL" />'
    );
  })();
</script>
```

`site-prefix.ts` already reads `document.querySelector('base').href`, so the
rest of the UI stack (`prefix-url.ts`, API calls, React Router `basename`)
continues to work without changes.

### API Calls Under a Proxy Prefix Rewrite (UC-3)

After the inline script runs, the UI builds API URLs by prepending the detected
prefix, e.g. `prefixUrl('/api/traces')` → `/foo/bar/api/traces`.  The browser
sends `GET /foo/bar/api/traces` to the proxy, which rewrites to `GET
/baz/api/traces` before forwarding to Jaeger.  Jaeger's route for `/baz/api/traces`
(registered via `--query.base-path=/baz`) matches correctly.

This means UC-3 works without any new backend flag:

| Layer | Configuration |
|---|---|
| Browser | accesses `/foo/bar/` — no config needed |
| Proxy | rewrites `/foo/bar/` → `/baz/` |
| Jaeger backend | `--query.base-path=/baz` |

The previously proposed `--query.ui-base-path` flag is **not needed**.

### Backend Changes

1. **Remove the base-path injection** from `static_handler.go`
   (`loadAndEnrichIndexHTML`).  The `basePathPattern` regexp and the replacement
   logic are deleted.

2. **Keep `--query.base-path` for API route registration only.**
   The flag still controls at which prefix the backend registers HTTP routes
   (e.g. `/baz/api/traces`).  Operators who use a non-root prefix must continue
   to set this flag so that API calls land on the correct handler.  For
   deployments where the ingress strips the prefix before forwarding, the flag is
   not needed at all.

### Handling Deep-Link Requests

The backend must still serve `index.html` for all SPA routes under the prefix so
that direct navigation (e.g. bookmarking `/jaeger/trace/abc123`) works.
This requirement is unchanged; `RegisterRoutes` already does this via a catch-all
handler.

### Development Mode

The Vite dev-server plugin (`vite.config.mts:jaegerUiConfigPlugin`) already
simulates backend behavior for local development.  Because the inline script
reads `window.location` directly, it works in the browser-based dev server
without any changes.  The `jaegerUiConfigPlugin` can be simplified to remove the
`<base>` injection it currently performs for dev mode.

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
`--query.base-path`.

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
* Backwards-compatible: existing deployments that set `--query.base-path`
  continue to work because API routes are still registered at the configured
  prefix.
* The previously proposed `--query.ui-base-path` flag is no longer needed.

### Negative

* The inline script must enumerate known top-level SPA route segments.  New
  routes must be added to both the router configuration and this list.
* The inline script is imperative JavaScript in `index.html`, which slightly
  complicates the HTML template.
* `--query.base-path` becomes partially redundant (needed only for API route
  registration, not UI delivery), which may confuse operators.  Clear
  documentation and a future deprecation path are required.

## Implementation Plan

### Step 1 — jaeger-ui: inline script in `index.html`

Replace the static `<base href="/" data-inject-target="BASE_URL" />` element in
`packages/jaeger-ui/index.html` with an inline `<script>` that:

1. Reads `window.location.pathname`.
2. Strips any known SPA sub-path suffix to isolate the mount prefix.
3. Writes `<base href="<prefix>">` into the document via
   `document.currentScript.insertAdjacentHTML('afterend', ...)`.

The script must appear **before** the `<link rel="shortcut icon">` and all other
tags so that the preload scanner uses the correct base URL for every subsequent
relative-URL reference.  The known sub-paths are the first path segment of every
registered top-level route:

| Route constant | Sub-path to strip |
|---|---|
| `searchPath` | `/search` |
| `tracePath` | `/trace/` |
| `dependenciesPath` | `/dependencies` |
| `deepDependenciesPath` | `/deep-dependencies` |
| `qualityMetricsPath` | `/quality-metrics` |
| `monitorATMPath` | `/monitor` |
| `plexusDemoPath` | `/plexus-demo` |

Root navigation (`/` and `/<prefix>/`) is handled by the existing React Router
redirects and does not require stripping.

Add a new test file `packages/jaeger-ui/src/utils/detect-base-path.test.ts`
(or inline in `prefix-url.test.js`) that validates prefix extraction for:
- root `/`
- each registered sub-path at root (e.g. `/search`, `/trace/abc`)
- each registered sub-path under a prefix (e.g. `/jaeger/search`,
  `/jaeger/trace/abc`)

`site-prefix.ts` requires **no changes**: it already reads
`document.querySelector('base').href`.

### Step 2 — jaeger-ui: export the sub-path list for testing

Extract the `knownSubPaths` array used in the inline script into a helper module
(`src/utils/detect-base-path.ts`) that can be imported by tests.  The inline
script in `index.html` uses a self-contained IIFE so it has no module
dependencies, but the same logic lives in the testable module.

### Step 3 — jaeger (backend): remove base-path injection from `static_handler.go`

In `cmd/jaeger/internal/extension/jaegerquery/internal/static_handler.go`:

- Delete the `basePathPattern` package-level variable.
- In `loadAndEnrichIndexHTML`: delete the entire "replace base path" block
  (lines 122–131 in the current code).
- The `BasePath` field and `--query.base-path` flag are **kept** — they continue
  to drive HTTP route registration in `RegisterRoutes`.
- The base-path validation (`HasPrefix` / `HasSuffix` check) moves out of
  `loadAndEnrichIndexHTML` into the `QueryOptions` validation step (or stays in
  `RegisterRoutes`), since it is still needed for route registration correctness.

### Step 4 — jaeger (backend): update `static_handler_test.go`

- Remove the `expectedBaseHTML` field from test cases (or keep it but change all
  four expected values to `<base href="/"` — the literal that now stays in the
  HTML untouched).
- Remove the invalid-base-path test cases from `TestNewStaticAssetsHandlerErrors`
  that currently test the injection validation; replace with validation tests at
  the `RegisterRoutes` / options level if the validation is moved there.

### Step 5 — documentation

Update `--query.base-path` help text to clarify that it controls API route
registration only, not UI delivery.  Update deployment docs / examples to show
the proxy-rewrite pattern (UC-3).

## Test Plan

### Unit tests (automated, in CI)

#### jaeger-ui: `detect-base-path`

`src/utils/detect-base-path.test.ts` — covers `detectBasePath()`:

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

#### UC-1: Direct access, `--query.base-path` matches ingress

Setup:
- Jaeger: `--query.base-path=/jaeger`
- NGINX: forwards `/jaeger/` to Jaeger without rewriting

Checks:
1. `GET /jaeger/` → `index.html` loads, inline script detects prefix `/jaeger/`.
2. Static assets (`/jaeger/assets/…`) return 200.
3. API calls go to `/jaeger/api/services` → 200.
4. Deep-link: navigate directly to `/jaeger/trace/<id>` in a fresh tab → page loads, assets 200, trace loads.
5. Browser back/forward between `/jaeger/search` and `/jaeger/trace/<id>` works.

#### UC-2: Same pod, two external prefixes

Setup:
- Jaeger: `--query.base-path=/` (root, or omitted)
- NGINX rule A: forwards `https://host-a/` → Jaeger `/`
- NGINX rule B: forwards `https://host-b/jaeger/` → Jaeger `/` (strips prefix)

Checks:
1. Both `https://host-a/search` and `https://host-b/jaeger/search` render correctly.
2. Deep-links work under both prefixes.
3. API calls from each origin go to the correct path relative to that origin.

#### UC-3: Proxy rewrites external prefix to a different internal prefix

Setup:
- Jaeger: `--query.base-path=/baz`
- Proxy: rewrites `/foo/bar/` → `/baz/` (strips `/foo/bar`, prepends `/baz`)

Checks:
1. `GET /foo/bar/` → `index.html` loads; inline script detects prefix `/foo/bar/`.
2. Static assets at `/foo/bar/assets/…` → proxy rewrites to `/baz/assets/…` → 200.
3. API calls: browser sends `GET /foo/bar/api/services` → proxy rewrites to `GET /baz/api/services` → 200.
4. Deep-link `GET /foo/bar/trace/<id>` → page loads, trace renders.
5. Repeat UC-1 checks (deep-link, back/forward) under the `/foo/bar/` external prefix.

### Regression checks

- `--query.base-path` omitted (root deployment): UI loads at `/`, all routes work.
- `--query.base-path=/jaeger` with no proxy (current common case): UI loads at `/jaeger/`, all routes work — identical behaviour to before this change.
- Hot-reload of `jaeger-ui.config.json` still works (backend still rewrites the config placeholder in `index.html`; only the `<base>` injection was removed).

## References

- `jaeger-ui/packages/jaeger-ui/index.html` – `<base>` tag definition
- `jaeger-ui/packages/jaeger-ui/src/site-prefix.ts` – runtime prefix detection
- `jaeger-ui/packages/jaeger-ui/src/utils/prefix-url.ts` – prefix application to URLs
- `cmd/jaeger/internal/extension/jaegerquery/internal/static_handler.go` – backend injection
- [jaeger-ui issue #42](https://github.com/jaegertracing/jaeger-ui/issues/42) – original motivation for `<base>` + relative paths
- [jaeger issue #5157](https://github.com/jaegertracing/jaeger/issues/5157) – feature request: support external URL prefix (proxy rewrite case)
- [jaeger PR #5219](https://github.com/jaegertracing/jaeger/pull/5219) – attempted `--query.ui-base-path` implementation (not merged)
