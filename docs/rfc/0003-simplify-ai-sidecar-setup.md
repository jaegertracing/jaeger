# RFC 0003: Simplify Running Jaeger With the AI Sidecar

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-06-06
- **Last Updated:** 2026-06-07

**Implementation status:** The UI half of §4.1 has landed in
[jaeger-ui#4034][pr-4034] — `backendCapabilities.aiAssistant` is now
the sole UI gate for the chat surface. The remaining work is in the
backend: the liveness probe (§4.1.2) and the
`JAEGER_BACKEND_CAPABILITIES` injection in `static_handler.go`.

---

## Abstract

Standing up Jaeger with the AI assistant enabled requires exporting a
provider API key and running two separate processes in the right order.
None of these steps carry interesting decisions for the operator — they
exist because each layer was built in isolation. This RFC proposes
collapsing the setup to one command and one secret, by:

1. Driving AI assistant visibility from a backend-advertised,
   liveness-checked `backendCapabilities.aiAssistant` capability, so the
   UI lights up automatically when a reachable sidecar exists;
2. Shipping per-sidecar single-command launchers (`make run-ai-<sidecar>`
   and matching compose files) that bring up Jaeger and the chosen
   sidecar together, with a shared convention so adding another sidecar
   is a small repeatable change; and
3. Considering — but **not** recommending in this RFC — letting the Jaeger
   binary supervise the sidecar as a child process (§5.4).

The first two are scoped as the concrete proposal; the third is captured as a
follow-up alternative.

---

## 1. Motivation

[RFC 0002][rfc-0002] established the AI gateway and a reference sidecar
implementation. The protocol design is sound, but the resulting operator
experience for a first-time local run is heavier than it needs to be.
The required steps are:

1. Pick a sidecar implementation, install its toolchain, and arrange
   provider auth (e.g. Python + `uv` + `GEMINI_API_KEY` for the Gemini
   reference; Node + `npm` + Claude Max login or `ANTHROPIC_API_KEY` for
   the Claude Code reference).
2. Start the Jaeger binary.
3. Start the sidecar.

(The `jaeger_query.ai` config block is the opt-in: when present in the
YAML, `configoptional.Default(...)` at [`flags.go:86-89`][flags-go]
fills in `agent_url: ws://localhost:16688`; when absent, the AI
extension is not wired up at all and the chat endpoint is not
registered. The embedded all-in-one config at
`cmd/jaeger/internal/all-in-one.yaml` omits the block; the example
`cmd/jaeger/config.yaml:43-44` includes it. The presence-of-block
semantic is also why a naive config-only gate doesn't work — see
§4.1.)

Step 3 is friction-prone: it's a separate process with its own
toolchain. The set of toolchains is also growing — the Gemini sidecar
uses Python/`uv`, the in-progress Claude Code sidecar
([PR #8631][pr-8631]) uses Node/`npm`, and further sidecars are likely.
The startup order is forgiving but not obvious, and a fresh contributor
hits it on every laptop reboot.

The cost is paid by every new contributor, every demo, and every CI job that
wants to exercise the chat path end-to-end. It is also paid every time the
docs need to explain "and now, in a second terminal…".

We want: **one config flag for the UI, one command per chosen sidecar,
provider auth handled the way the provider expects.**

---

## 2. Scope and Non-Goals

**In scope:**

- Removing the duplicated UI-side enablement gate by deriving it from the
  backend.
- Single-command local launchers — one per reference sidecar — that start
  Jaeger and the chosen sidecar together with a single auth input
  appropriate to the provider.
- A shared launcher convention so that adding a third or fourth sidecar in
  the future does not require redesigning the entry point.
- Updating the existing READMEs (`scripts/ai-sidecar/README.md`, the
  per-sidecar READMEs under it, and the gateway README) to point at the
  new entry points.

**Out of scope:**

- Changing the wire protocol between gateway, sidecar, MCP server, or
  browser. [RFC 0002][rfc-0002] is unaffected.
- Choosing a single canonical sidecar. The launcher convention treats every
  reference sidecar under `scripts/ai-sidecar/<name>/` as equal; operators
  pick which one to run.
- Production deployment topology (Kubernetes manifests, Helm charts, etc.).
  This RFC is about local-dev/demo ergonomics.

**Under consideration (deferred — see §5.4):**

- In-process supervision: have the Jaeger binary spawn and supervise the
  sidecar when the operator opts in via config. Discussed as an alternative
  but **not** recommended in this RFC.

---

## 3. Background

### 3.1 Current moving parts

```
Browser ──HTTP──> Jaeger Query :16686 ──WS(ACP)──> Sidecar :16688 ──HTTPS──> LLM provider
                          │                              │
                          └─ MCP server :16687 <─────────┘
```

Five things have to be true for the chat surface to light up:

| # | What                                       | Source of truth                                                  | Status                                                  |
|---|--------------------------------------------|------------------------------------------------------------------|---------------------------------------------------------|
| 1 | `jaeger_query` extension running           | `service.extensions` in the embedded `internal/all-in-one.yaml`         | Already there                                           |
| 2 | `jaeger_query.ai` config block present     | Operator's YAML (see [`flags.go:86-89`][flags-go] for default values)   | Omitted by embedded config; present in `config.yaml`    |
| 3 | `jaeger_mcp` extension running             | `service.extensions` in the embedded `internal/all-in-one.yaml`         | Already there                                           |
| 4 | `backendCapabilities.aiAssistant` true     | Backend injection into `index.html` (to be implemented — §4.1)          | Automatic when backend half of §4.1 ships               |
| 5 | Sidecar process listening                  | Separate sidecar process under `scripts/ai-sidecar/<name>`              | Operator must run a second command                      |

Items 1 and 3 are satisfied by the embedded all-in-one config
(`cmd/jaeger/internal/all-in-one.yaml`); they hold for a bare
`go run ./cmd/jaeger` with no `--config` flag. Item 2 is **not**
satisfied by the embedded config — the `ai:` block under `jaeger_query`
is omitted there, so AI is off unless the operator passes
`--config cmd/jaeger/config.yaml` (which includes the block) or writes
their own config with the block. This is the backend-level opt-in;
see §3.2.

### 3.2 Operator opt-in is expressed by the backend setup itself

AI assistance is a serious operator decision: it requires provisioning
LLM API keys, running a sidecar process, accepting data-egress to a
third-party model, and potentially paying for tokens. A Jaeger
installation should never light up the chat surface unless the operator
has *actively chosen* to enable it.

That opt-in is fully expressed by the act of standing up a sidecar. An
operator who has:

1. Added the `jaeger_query.ai` block to the backend config (or picked a
   config that already includes it), and
2. Stood up a sidecar that's actually reachable, and
3. Provisioned the relevant LLM credentials for that sidecar to use,

…has unambiguously opted in. Those three actions cost real effort and
will not happen by accident. The first one alone is already a hard gate
on the backend side: `configoptional.Default(...)` on `AIConfig` means
the AI extension is wired up only if the block is present in YAML, and
the chat endpoint is registered only when `aiCfg` is non-nil.

The liveness probe (§4.1.2) is the honest expression of full opt-in:
the capability is true exactly when the operator has done all three
*and* the result is functional. False otherwise — whether because they
haven't opted in, the sidecar isn't running, or something is broken.
The UI just needs to reflect that.

---

## 4. Proposed Design

### 4.1 Make AI availability a backend-derived, liveness-checked capability

Treat AI availability the same way the UI treats archive storage and
metrics storage: it's a **capability** the backend advertises, not a
configuration the UI re-declares. But unlike storage — which is wired into
the same process and either works or fails at boot — the AI gateway depends
on an *external* sidecar that may or may not be running. The
`jaeger_query.ai` block being present in the config tells us the
operator *intends* to enable AI (and, via `configoptional.Default`,
gives us a defaulted `agent_url`), but not that the sidecar at the
other end is actually reachable. A capability derived purely from the
config block's presence would advertise a feature the operator can't
actually use any time they have the example `config.yaml` loaded but
no sidecar running — the UI would render the chat surface and every
call would fail.

The design therefore needs two things: a separate namespace for future
gates (existing `storageCapabilities` is too narrow), and a **liveness
probe** so the capability reflects the sidecar's real availability.

#### 4.1.1 New `backendCapabilities` namespace

The existing `storageCapabilities` blob ([`static_handler.go:113-116`][static-handler])
holds two storage-specific flags today (`archiveStorage`,
`metricsStorage`). The name was picked when storage was the only thing
the backend needed to advertise to the UI; in retrospect a more general
name like `backendCapabilities` would have left room for non-storage
flags. AI availability is the first such flag we want to ship — adding
it as `storageCapabilities.aiAssistant` would lock in the misnomer. This RFC
therefore introduces a parallel `backendCapabilities` blob:

```jsonc
{
  "aiAssistant": true         // sidecar reachable right now
  // future: "alerting": …, "compare": …, etc.
}
```

Injected the same way `storageCapabilities` is — index.html regex
replacement at static-handler boot, exposed as
`window.getJaegerBackendCapabilities`. The UI side already exists
(PR #4034): the getter is wired up, the `aiAssistant` field is typed,
and the chat surface gates on it. Storage flags stay on the legacy
`JAEGER_STORAGE_CAPABILITIES` injection for now; the UI composes them
into the unified `backendCapabilities` blob at read time, so the
backend can start writing the new `JAEGER_BACKEND_CAPABILITIES` pattern
without re-implementing storage advertisement.

Migrating `storageCapabilities` into `backendCapabilities` is a
separate cleanup, out of scope here. Once the backend writes
`JAEGER_BACKEND_CAPABILITIES` with storage flags included, the legacy
pattern can be retired without further UI changes.

#### 4.1.2 Liveness probe

When the `jaeger_query.ai` config block is absent, the checker is
not started at all and `aiAssistant` is statically `false` — there's
nothing to check. When the block is present, the backend runs a small
loop that periodically opens an ACP WebSocket to `agent_url`,
completes the `initialize` handshake, and closes. Each check records
its result into an atomic boolean. The loop:

- Default interval: **5 s**. Configurable via
  `jaeger_query.ai.health_check_interval` for tuning. `0` (the zero
  value) selects the default in keeping with Go's `time.Duration`
  convention; a negative duration is an optional within-block disable
  signal for config templates that always include the `ai:` block.
  The operator-facing way to disable AI is to omit the `ai:` block
  entirely (see §3.2).
- Default per-check timeout: **2 s**. Configurable via
  `jaeger_query.ai.health_check_timeout`.
- Probe payload: a minimal ACP `initialize` request. No `session/new` or
  `prompt` — initialize is the cheapest reachability test that exercises
  the actual protocol path the chat handler uses, not just a TCP dial.
- Failure handling: any transport error or non-success ACP response
  yields `aiAssistant = false` immediately. We don't smooth across N
  failures; a single probe failure flips the state, because the chat call
  would also fail.
- **Initial state**: `false` until the first probe completes. We
  under-promise rather than over-promise; the alternative (advertise
  `true` on boot and downgrade on first probe failure) creates a brief
  window where the UI lights up the chat surface and immediate clicks
  fail.

When the probe result changes, the static handler regenerates
`indexHTML` and atomically swaps it via the existing
`indexHTML.Store(...)` pattern that already supports fswatcher-driven
config reloads ([`static_handler.go:97,133`][static-handler]). A user
who started Jaeger before the sidecar gets the chat surface on their
next page reload, with no further action.

The probe is a thin client implemented against the same ACP library the
chat handler uses, so it shares the connection semantics (TLS, headers,
auth — none today, but extensible). It does **not** share the chat
handler's per-session connection; each probe is its own short-lived
connection.

#### 4.1.3 Capability delivery mechanism

Once §4.1.2's probe produces a live `backendCapabilities` state on the
backend, that state has to reach the UI. There are three places to land
on the mechanism spectrum. Each composes with the same probe and the
same `backendCapabilities` blob — the differences are in *how* the UI
learns and *when* it can update.

##### Option 1 — Static injection (extends today's pattern)

Backend writes the `backendCapabilities` blob into `index.html` at
asset-handler boot, and re-writes it via the existing atomic-swap path
([`static_handler.go:97,133`][static-handler]) whenever the probe state
flips. UI reads `window.getJaegerBackendCapabilities()` synchronously
during boot. Same shape and pattern as `storageCapabilities` today.

*Pros:*

- **Synchronous, deterministic first paint.** A user deep-linking to a
  capability-gated page (e.g. `/monitor`) knows at mount time whether to
  render the splash or the real view. No race, no flash.
- **No new HTTP endpoint, no new UI infrastructure.** Reuses the
  existing static-handler plumbing and the `window.getJaeger*` getter
  convention.
- **Zero impact on cold-start latency.** The capability rides along with
  the HTML the browser is already fetching.
- **No behavioral change for existing UI consumers** of
  `storageCapabilities`.

*Cons:*

- **Stale until reload.** A sidecar that comes up after Jaeger requires
  the user to refresh the page before the chat surface appears.
- **Couples the static handler to the AI checker.** The handler needs
  to either subscribe to check state changes or call `Current()` at
  serve time (the latter is what the implementation ended up doing).
- **Every future dynamic-state capability we add inherits the same
  reload-to-update behavior.** Survivable for one capability; less great
  as the list grows.

##### Option 2 — Pure async via a `useConfig`-style hook

Backend exposes `GET /api/capabilities` returning the
`backendCapabilities` blob in JSON. UI fetches on mount and exposes via
a `useCapabilities` (or extended `useConfig`) hook. Components subscribe
and re-render when the hook's state changes. The static injection of
`storageCapabilities` is retired; everything flows through the async
channel.

*Pros:*

- **Live updates without reload.** Sidecar restarts, storage failover,
  anything else that changes capabilities — the UI sees it within a
  polling interval (or push event).
- **Decouples runtime state from static assets.** The asset handler no
  longer needs to know about background checkers.
- **Aligns with the existing direction** of moving components onto the
  `useConfig` hook: one centralized capability source, all consumers
  subscribe, dynamic updates are free.
- **Opens the door to richer payloads** (per-feature status, error
  messages, last-checked timestamps) without growing `index.html` size.

*Cons:*

- **First-paint race for capability-gated pages.** A deep-link to
  `/monitor` mounts before `/api/capabilities` resolves, forcing an
  unsatisfying choice per surface:
  - *Loading spinner* — honest but adds first-paint latency to every
    capability-gated page, even when the gate would have allowed
    immediate render.
  - *Optimistic render + correct* — `/monitor` renders real content,
    flashes to the splash if metrics turn out unavailable.
  - *Pessimistic render + correct* — `/monitor` shows the splash, flashes
    to real content if metrics are available.

  Static injection ducks this entirely; pure async makes it a
  per-surface UX policy.
- **Extra round-trip on initial page load** (small but non-zero).
- **Requires migrating `storageCapabilities`** to the same channel, or
  accepting two delivery mechanisms indefinitely.
- **More test surface:** every consumer of the hook must behave
  correctly under "no data yet → data" transitions.

##### Option 3 — Hybrid: static seed + async revalidate

Backend serves both:

- Static injection (Option 1's mechanism) for the initial render — same
  blob, same regex pattern, same atomic-swap on probe state change.
- `GET /api/capabilities` returning the same blob in JSON.

UI consumes via a `useCapabilities` hook that:

- Returns `window.getJaegerBackendCapabilities()` synchronously for the
  first paint (the seed).
- Issues an async fetch in the background and updates state if the
  response disagrees with the seed.
- Re-fetches on natural events: tab visibility return, manual refresh
  action (e.g. clicking the chat icon), or a low-frequency poll while
  the page is open.

*Pros:*

- **Deterministic first paint** (seeded). No `/monitor` splash flash, no
  spinner on capability-gated pages.
- **Live updates for the scenarios that matter:** sidecar comes up while
  the user has the page open; storage failover during a long session.
- **Composes additively with Option 1.** Same probe, same blob, same
  field name. Adding the async endpoint and hook does not change the
  static-injection path.
- **Migration of `storageCapabilities`** (and other future capabilities)
  to the hybrid model is mechanical once the channel exists.

*Cons:*

- **Two delivery channels for the same data.** Divergence is possible:
  the static seed may lag the async response by up to one re-fetch
  interval.
- **Re-render policy still required** when async disagrees with seed
  mid-session. The chat sidebar appearing/disappearing is cheap, but a
  `/monitor` tab swapping its content while the user is on it needs a
  graceful path (e.g., toast + opt-in re-navigate, rather than a hard
  swap).
- **More moving parts:** new endpoint, new hook, re-fetch trigger
  policy.

#### 4.1.4 Phasing: Option 1 now, Option 3 later

This RFC commits to **Option 1 for the immediate work**, with
**Option 3 as the destination**. The reasoning:

- Option 1 already builds **the entire backend half of Option 3** — the
  probe, the data shape, the `backendCapabilities` blob, the field
  name, the atomic-swap pattern, the field-name conventions. None of
  that has to change when we add the async endpoint later.
- Option 1 ships the operator-facing win (no UI gate to edit, no
  reload-after-config-change confusion) in a single PR, against
  existing UI infrastructure.
- The "needs reload" downside of Option 1 is small for the AI surface
  specifically: the chat icon is a sidebar; the operator simply
  refreshes once after starting the sidecar.
- Option 3 needs follow-up design work (re-fetch triggers, hook
  contract, per-surface re-render policies for capability-gated tabs
  like `/monitor`) that benefits from a separate RFC. Squeezing it
  in here would either delay the operator-facing fix or under-specify
  the hybrid.

**Migration to Option 3** later requires only adding `GET
/api/capabilities` and the UI hook; the backend probe and the static
injection stay as-is (or the static path retires once the seed-first
case is judged less valuable than fewer-channels). Either way, Option 1
is foundation, not throwaway.

The remaining design work for Option 3 is captured in §7.4–§7.7 as
explicit open questions to resolve when that RFC is written.

### 4.2 Single-command local launcher (per sidecar)

Each reference sidecar gets a Make target named after the sidecar
directory. The target encapsulates the toolchain bootstrap, the Jaeger
binary launch, and the sidecar launch, in the right order:

```bash
# Gemini
export GEMINI_API_KEY=…
make run-ai-gemini

# Claude Code (API key path)
export ANTHROPIC_API_KEY=sk-…
make run-ai-claude-code

# Claude Code (Claude Max login path)
(cd scripts/ai-sidecar/claude-code && npm run auth:max)   # one-time
make run-ai-claude-code
```

Each target:

1. Bootstraps the sidecar's toolchain idempotently (`uv sync` for Gemini,
   `npm ci` for Claude Code). Fast on subsequent runs.
2. Validates the auth prerequisite for that sidecar (see §4.3).
3. Starts Jaeger in the background with the embedded all-in-one config.
4. Starts the sidecar in the foreground (Ctrl-C exits both).
5. Optionally waits for a "listening on …" log line before declaring
   readiness.

A shared shell helper (`scripts/ai-sidecar/_lib.sh` or similar) factors
out the Jaeger-launch + readiness-wait + cleanup-on-exit logic so the
per-sidecar Make targets stay short and adding a new sidecar is a
small, repeatable change.

For Docker-first operators, two compose files under
`docker-compose/ai-gemini/` and `docker-compose/ai-claude-code/`
mirror the Make targets. Each references the published
`jaegertracing/jaeger` image plus a small per-sidecar image built from
a `Dockerfile` colocated with the sidecar source. Compose files map
`:16686`, `:16687`, `:16688`, and `:4317` and pass the relevant auth
env var through.

Neither path replaces the existing two-terminal workflow; both are additive.

### 4.3 Provider-specific prerequisite check

Each Make target performs a small pre-flight specific to its sidecar's
auth model:

| Sidecar         | Pre-flight                                                                                   |
|-----------------|----------------------------------------------------------------------------------------------|
| Gemini          | `GEMINI_API_KEY` env var present and non-empty.                                              |
| Claude Code     | Either `ANTHROPIC_API_KEY` present **or** a valid Claude Max session in `~/.claude/` exists. |
| (future sidecar)| Defined by the sidecar's README; the launcher just calls a hook.                             |

When the pre-flight fails, the launcher exits with a one-line message
quoting the exact env var or login command, rather than the toolchain
stack trace operators see today. The check lives in the shared shell
helper so it is consistent across sidecars; each sidecar contributes a
tiny pre-flight script in its directory (`preflight.sh` or similar).

---

## 5. Design Alternatives Considered

### 5.1 Leave the UI gate, document it better

Add a prominent note to the gateway README saying "don't forget step 4".
**Rejected** because the gate's original justification (no backend shipping)
is gone, so the gate is now actively misleading rather than merely
under-documented. Documentation can't fix a config that asks the operator to
repeat themselves.

### 5.2 UI probes `/api/ai/chat` directly

The UI could issue a HEAD/OPTIONS to `/api/ai/chat` on load and enable the
chat surface if it returns 200/204, with no backend changes at all.

**Rejected** because the chat endpoint registration says nothing about
sidecar reachability — it only proves `agent_url` was non-empty in the
config. The UI would re-encounter exactly the failure mode §4.1 is
designed to prevent: chat surface lights up, first call fails. The
backend-side liveness probe is non-negotiable; given that, the
capabilities channel is the natural place to ship its result.

### 5.3 Capability derived from config, no liveness probe

Simpler implementation: advertise `aiAssistant = true` whenever the
`jaeger_query.ai` block is present in the config (or equivalently,
whenever `agent_url != ""` after `configoptional` defaulting). No
background checker, no per-request derivation, no interval to tune.

**Rejected** because it lies any time the operator's config has the
block but the sidecar isn't running — which is the *default* state
when using the shipped example `cmd/jaeger/config.yaml` without
starting a sidecar. The UI would render the chat surface and every
call would fail. A liveness check is cheap (one ACP `initialize` every
5 s) and removes an entire class of "I deleted the sidecar but the
chat surface is still there" bug reports.

### 5.4 In-process sidecar supervision

Let the Jaeger binary spawn the sidecar as a child process when the
operator opts in (e.g. via a new `jaeger_query.ai.spawn: gemini`
field). The Go side would `exec.Command` `uv run python main.py` (or a
packaged binary), pipe logs, and propagate signals.

**Attractive because** it collapses the run-Jaeger and run-sidecar steps
into one process invocation — closest possible to "one command, no
launcher".

**Rejected for this RFC because:**

- It bakes a Python toolchain assumption into a Go binary. Distro packagers
  would object, and CI images would need Python.
- It blurs the layering RFC 0002 deliberately preserved: the gateway speaks
  ACP over WS to *some* agent, and ACP agnostic-ness is a feature.
- Process supervision (restart-on-crash, log routing, shutdown ordering) is
  a non-trivial new responsibility for the Jaeger binary.
- The launcher in §4.2 captures most of the ergonomic benefit at a tiny
  fraction of the engineering cost.

Worth revisiting if and when the project ships a self-contained Go-native
sidecar; until then, the launcher is the better lever.

### 5.5 Default the launcher into the binary's `--dev` mode

Add a `--dev-ai` flag to `cmd/jaeger` that, when set, prints the exact
sidecar command to run in another terminal and exits. **Rejected** as
strictly worse than §4.2: a flag that prints a command is not a launcher.

---

## 6. Implementation Plan

Two PRs, each independently shippable.

### PR1 — Backend-derived AI capability with liveness probe

UI side (`jaeger-ui/`) is already done in [PR #4034][pr-4034]: the
`backendCapabilities` namespace, `aiAssistant` field, and chat-surface
gate all exist. What remains is backend.

Backend (`cmd/jaeger/internal/extension/jaegerquery/`):

- New `JAEGER_BACKEND_CAPABILITIES` injection in `static_handler.go`,
  parallel to the existing `JAEGER_STORAGE_CAPABILITIES` replacement at
  lines 113–116. The injected JSON includes `aiAssistant` and (for
  future-proofing) `archiveStorage` / `metricsStorage` mirrored from
  `storageCapabilities`, so the legacy storage pattern can be retired
  cleanly once both backends and UIs are upgraded.
- New `jaegerai/aihealth` package implementing the periodic ACP
  `initialize` check described in §4.1.2. Exposes `Current() bool`; the
  ACP-specific check function is `aihealth.NewACPCheck(agentURL, logger)`.
- The static handler derives `index.html` per request, calling
  `aiHealthCheck()` inline so the injected `aiAssistant` flag always
  reflects the latest health-check value (no subscribe / cache-invalidate
  machinery).
- New config fields `jaeger_query.ai.health_check_interval` (default
  `5s`) and `jaeger_query.ai.health_check_timeout` (default `2s`) in
  `flags.go` alongside the existing `AgentURL`.

Docs:

- Update `scripts/ai-sidecar/README.md`'s "Verify It Works" section to
  describe the new automatic-capability flow.

### PR2 — Shared launcher convention + per-sidecar targets

- New `scripts/ai-sidecar/_lib.sh` (or equivalent) holding the shared
  Jaeger-launch, readiness-wait, cleanup-on-exit, and pre-flight-dispatch
  logic.
- Per-sidecar Make targets at the top-level Makefile near existing `run-*`
  targets — initially `run-ai-gemini` and `run-ai-claude-code`. Adding
  a future sidecar means adding one Make target and one `preflight.sh`,
  no changes to shared infrastructure.
- New `Dockerfile`s colocated with each sidecar's source
  (`scripts/ai-sidecar/gemini/Dockerfile`,
  `scripts/ai-sidecar/claude-code/Dockerfile`). Per-sidecar compose files
  under `docker-compose/ai-<sidecar>/docker-compose.yml` reference the
  published Jaeger image plus the local sidecar image.
- README updates pointing at the new Make targets.

The two PRs are independent: §4.1 stands on its own; §4.2 is useful even if
§4.1 isn't merged yet.

---

## 7. Open Questions

### 7.1 Probe interval and timeout defaults

`5 s` / `2 s` are educated guesses. Too aggressive wastes connections;
too lazy delays UI recovery after a sidecar restart. A few questions:

- Should the interval back off when the sidecar is reachable (cheap to
  poll a healthy thing infrequently) and accelerate when it's unreachable
  (so the UI lights up faster after the user fixes things)? Probably
  yes, but adds state. Acceptable to start with a flat interval.
- The documented way to disable AI is to omit the `jaeger_query.ai`
  block from the config — that already skips probing and emits
  `aiAssistant: false` per §4.1.2. Should we additionally honor an
  explicit `agent_url: ""` as a within-block disable (useful for
  config templates that always include the block)? Probably yes, with
  identical semantics to omitting the block.
- Should the probe be skipped if the static handler hasn't loaded
  index.html yet? No — initial-probe latency is the dominant factor in
  "how long until the UI shows AI is on" after both processes start.

### 7.2 Should the launchers pre-fetch sidecar Docker images?

The first `make run-ai-<sidecar>` will build the sidecar image (a few
seconds on warm cache, longer on cold). We could push prebuilt images
(`jaegertracing/ai-sidecar-gemini:latest`,
`jaegertracing/ai-sidecar-claude-code:latest`) to ghcr.io and pull
instead. Defer until §4.2 lands and we see real cold-start times.

### 7.3 Discovery: should `make run-ai` exist as an alias?

With per-sidecar targets, a contributor who types `make run-ai` (no
suffix) hits nothing. Options:

- Leave it unset; rely on `make help` and the READMEs.
- Make `run-ai` an alias for the first listed sidecar (today: Gemini).
  Discoverable but biases the operator.
- Make `run-ai` print the list of available sidecars and exit
  non-zero. Friendly nudge without picking favorites.

Suggested: print-and-list. Cheap, surfaces the choice, never silently
runs the wrong thing.

---

The following questions belong to the **Option 3 follow-up RFC**, captured
here so the design space is on the record but explicitly out of scope for
the immediate work.

### 7.4 Re-fetch trigger policy (Option 3)

Open: which combination of triggers should the `useCapabilities` hook
use?

- Tab visibility return (cheap, picks up most "I started the sidecar in
  another terminal and tabbed back" cases).
- Periodic poll (interval TBD — 30 s? 60 s? Probably no need for sub-30 s
  given the backend probe interval).
- On user action — e.g. clicking the chat icon issues an immediate
  re-fetch and shows "checking…" state if the cached seed says
  unavailable.
- Server push (SSE/WS) — most elegant but adds a new long-lived channel
  the UI must manage.

The right answer is probably "visibility + a 60 s poll while visible +
on-click for the chat surface specifically," but worth designing
carefully.

### 7.5 Per-surface re-render policy when seed and async disagree (Option 3)

Open: how should each capability-gated surface behave when an async
revalidate flips its gate after mount?

- **Chat sidebar (AI):** appearing-then-hiding is cheap; both directions
  are acceptable as silent updates.
- **`/monitor` tab:** mid-session swap from "real view" to "splash" is
  jarring. Suggested policy is a one-time toast ("metrics storage is
  no longer available — back to the monitor list?") rather than a hard
  swap.
- **Tab visibility in the nav bar:** showing/hiding nav entries
  mid-session is more disruptive than a content area swap; possibly
  freeze the nav until the next full page load.

Resolving this likely means classifying each gated surface and writing
a small policy table.

### 7.6 Endpoint shape (Option 3)

Open: is `GET /api/capabilities` the right URL and payload? The blob
inside `backendCapabilities` is the natural payload, but other options
include returning both `storage` and `backend` capabilities together,
returning a richer shape with per-feature timestamps and probe error
messages, or namespacing under `/api/v1/capabilities` if we want to
version it from day one.

### 7.7 `storageCapabilities` migration (Option 3)

Open: when Option 3 lands, does `storageCapabilities` migrate to the
async channel (one source of truth, no static injection for storage
flags) or stay statically injected as today? The pure-async direction
is cleaner but reintroduces the `/monitor` splash race for an existing
gate that doesn't have it today. Worth deciding deliberately rather
than by default.

Note: PR #4034 already structurally folded `storageCapabilities` into
the UI's `backendCapabilities` view, so on the UI side the migration is
about retiring the legacy `JAEGER_STORAGE_CAPABILITIES` injection once
backends are advertising storage flags via the unified blob. The
async-channel question is orthogonal to that mechanical cleanup.

---

## 8. Appendix — Before/After

### 8.1 Today (Gemini example; Claude Code is analogous with `npm` + `ANTHROPIC_API_KEY`/Claude Max)

```bash
# terminal A:
go run ./cmd/jaeger --config cmd/jaeger/config.yaml

# terminal B:
cd scripts/ai-sidecar/gemini
uv sync
export GEMINI_API_KEY=…
uv run python main.py
```

Until the backend half of §4.1 ships, `backendCapabilities.aiAssistant`
defaults to `false`, so the chat surface stays hidden. For local
development, operators can override the default in
`jaeger-ui.config.json` with
`{ "backendCapabilities": { "aiAssistant": true } }`.

### 8.2 With this RFC adopted

```bash
# Pick a sidecar and run its target.
export GEMINI_API_KEY=…           # or ANTHROPIC_API_KEY=…
make run-ai-gemini                # or make run-ai-claude-code
```

…and the UI chat surface lights up automatically because the backend told
it to. The Make target handles toolchain bootstrap (`uv sync` / `npm ci`),
pre-flight auth check, Jaeger launch, sidecar launch, and clean shutdown.

---

[rfc-0002]: ./0002-ai-gateway-contextual-tools.md
[sidecar-readme]: ../../scripts/ai-sidecar/README.md
[ui-config]: ../../jaeger-ui/packages/jaeger-ui/src/types/config.ts
[server-go]: ../../cmd/jaeger/internal/extension/jaegerquery/internal/server.go
[flags-go]: ../../cmd/jaeger/internal/extension/jaegerquery/internal/flags.go
[static-handler]: ../../cmd/jaeger/internal/extension/jaegerquery/internal/static_handler.go
[pr-8631]: https://github.com/jaegertracing/jaeger/pull/8631
[pr-4034]: https://github.com/jaegertracing/jaeger-ui/pull/4034
