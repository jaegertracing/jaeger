# RFC 0018: Streaming Responses in the API v3 HTTP Gateway

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-09-22
- **Last Updated:** 2026-09-22
- **Issue:** [jaegertracing/jaeger#6467](https://github.com/jaegertracing/jaeger/issues/6467)
- **Related:** [RFC 0011 (trace summary API)](0011-trace-summary-api.md), [RFC 0014 (search result pagination)](0014-search-result-pagination.md), [RFC 0016 (span search)](0016-span-search.md)

---

## Abstract

The API v3 gRPC service streams its results: `GetTrace`, `FindTraces`, `FindTraceSummaries` and `FindSpans` are all server-streaming RPCs, and the query service feeds them from a lazy `iter.Seq2` that the storage backends produce chunk by chunk. The HTTP gateway that mirrors that service throws the streaming away. It drains the whole iterator into memory, merges every chunk into one OTLP document, and writes it as a single JSON object, so a client sees nothing until the last span has been read from storage.

This RFC proposes one streaming convention for every server-streaming method of the API v3 HTTP gateway. A client opts in per request with an `Accept` header; the default response stays the single JSON document that existing clients parse today. The body of a streaming response is a sequence of JSON objects, one per gRPC stream message, in the same `{"result": …}` envelope the gateway already uses, and every stream ends with exactly one terminal object: `{"end": true}` on success, or `{"error": …}` for a failure that occurs after the response has started. Three framings for that sequence are compared, newline-delimited JSON (NDJSON), Server-Sent Events (SSE) and gRPC-Web, and NDJSON is recommended. The RFC also settles the two questions the issue raised alongside streaming: how the response interacts with compression (whole-stream gzip that the server flushes after each message) and how it is carried over HTTP/1.1 and HTTP/2 (the standard library frames it, and the handler never touches `Transfer-Encoding`).

## 1. Motivation

### 1.1 The gateway buffers what the rest of the system streams

The storage API v2 reader returns traces as `iter.Seq2[[]ptrace.Traces, error]` (`internal/storage/v2/api/tracestore/reader.go`), with the rule that a chunk never mixes traces but a large trace may be split across consecutive chunks. The query service preserves that shape, and by default aggregates the chunks back into whole traces before adjusting them (`QueryService.receiveTraces` in `cmd/jaeger/internal/extension/jaegerquery/querysvc/service.go`). The gRPC handler then sends one message per `ptrace.Traces` element it receives (`receiveTraces` in `cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/grpc_handler.go`), so a gRPC client starts receiving the first trace while the backend is still fetching the last one.

The HTTP gateway (`cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/http_gateway.go`, called `http_gateway.go` below) sits on the same query service but calls `jiter.FlattenWithErrors` on the iterator, which collects every chunk into a slice and returns the first error, if any, in place of all of it. `returnTraces` then copies every trace's `ResourceSpans` into one `ptrace.Traces` and marshals it once. The consequences are the ones the issue lists:

- **Server memory.** The whole result, plus its merged copy, plus its JSON encoding, is held at once. A `FindTraces` for a busy service with a generous limit, or a `GetTrace` for a trace with tens of thousands of spans, allocates all of it before the first byte leaves the process.
- **Client latency.** Time to first byte equals time to last byte. Jaeger UI, which is in the middle of moving from the legacy `/api/` endpoints to `/api/v3`, cannot render anything until the entire document has arrived and been parsed.
- **A gRPC and HTTP asymmetry.** The proto comment on `GRPCGatewayWrapper` already promises that "in the future the server may return multiple responses using `Transfer-Encoding: chunked`". Clients on the two transports currently get different guarantees from the same RPC.

### 1.2 What the earlier attempt showed

[PR #7555](https://github.com/jaegertracing/jaeger/pull/7555) implemented streaming by writing each trace as an element of a JSON array, `[` then trace, `,` trace, `]`, flushing after each. It was closed for reasons that shape this design:

- It changed the response shape for every client, turning the top-level object into an array, which is a breaking change to a stable API.
- A JSON array is not incrementally parseable with `JSON.parse` or `encoding/json` without a custom tokenizer, so the client still had to read the whole body before it could use any of it.
- It had no answer for an error raised after the `200` status had been sent, and no answer for compression.
- Its tests asserted on substrings of the recorded body rather than demonstrating how a client consumes the stream.

An [ADR draft posted on the issue](https://github.com/jaegertracing/jaeger/issues/6467) later proposed gRPC-Web framing with an `Accept`-header opt-in. The opt-in is adopted here; the framing and the trace-level granularity it required are evaluated in §5 and §6 rather than assumed.

## 2. Goals and non-goals

**Goals**

- **G1.** A client that asks for it receives the results of a server-streaming API v3 RPC incrementally over HTTP, with the same message boundaries the gRPC stream has, and can parse each message as soon as it arrives.
- **G2.** Clients that do not ask for it observe no change: same status codes, same body, same `Content-Type`. The one deliberate exception is the `FindTraces` empty-result code (§6.6).
- **G3.** A failure after the response has started is reported in the stream with the same error shape the gateway uses today, a successful stream ends with an explicit marker, and a client can tell a complete stream from a failed or truncated one.
- **G4.** The format is consumable from a browser with `fetch` and from Go with `net/http`, with no client library and no new npm dependency.
- **G5.** Streaming responses can be compressed without buffering the whole stream on either side.
- **G6.** The convention applies to every current and future server-streaming RPC in API v3, not only to the two trace endpoints.

**Non-goals**

- Changing the gRPC service or its proto definitions.
- Adding HTTP/2 without TLS (h2c) or any other transport the query server does not offer today.
- Registering the routes the proto declares but the handwritten gateway does not serve: the `POST` bindings of `/api/v3/traces`, `/api/v3/trace-summaries` and `/api/v3/spans`, and `GET /api/v3/dependencies`.
- Changing how the query service aggregates or adjusts traces.

## 3. Current state

| Aspect | Today |
| --- | --- |
| HTTP routes | `GET` only, on a stdlib `http.ServeMux`: `/api/v3/traces/{trace_id}`, `/api/v3/traces`, `/api/v3/trace-summaries`, `/api/v3/services`, `/api/v3/operations`; `/api/v3/spans` is added by [#9613](https://github.com/jaegertracing/jaeger/pull/9613). |
| Success body | For the trace endpoints, one `GRPCGatewayWrapper` object, `{"result": {"resourceSpans": […]}}`, marshalled with gogo `jsonpb`; the inner OTLP JSON comes from `ptrace.JSONMarshaler`. For `/trace-summaries`, the `FindTraceSummariesResponse` as the top-level object with no `result` wrapper (§6.1). `Content-Type: application/json`. |
| Error body | `{"error": {"httpCode": N, "message": "…"}}` written through `http.Error`, so with `Content-Type: text/plain`. 404 for a missing trace and, unlike the gRPC handler and the other search endpoints, for an empty `FindTraces` result; 400 for bad parameters, 403 when a query interceptor denies access (`queryinterceptor.ErrAccessDenied`), 500 otherwise. Tenancy failures are answered with `401` by the tenancy middleware before the gateway runs. |
| Content negotiation | None. The `Accept` header is ignored. |
| Response compression | None. `confighttp.ServerConfig` (collector v0.160.0) only decompresses request bodies; nothing in the `jaegerquery` handler chain compresses responses. The UI's static assets are embedded gzipped and `internal/gzipfs` decompresses them when opened, so `http.FileServer` serves them as plain bytes with no `Content-Encoding`. |
| Transport | HTTP/1.1 in plaintext. With TLS, `confighttp` advertises `h2` and `http/1.1` via ALPN, so an HTTP/2-capable client gets HTTP/2. There is no h2c. |
| Timeouts | `write_timeout` and `idle_timeout` are operator-configurable and the query extension's default configuration leaves them unset. `confighttp.NewDefaultServerConfig` sets `write_timeout` to 30 seconds, so the extension must keep building its own default rather than adopt that constructor, or every stream longer than 30 seconds would be cut. |
| Streaming elsewhere | The AI assistant chat endpoint (`cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/endpoint_chat.go`) streams AG-UI events as SSE and the UI consumes them through `@ag-ui/client`. That format is specific to AG-UI and is not reusable for query results. |
| UI consumers | `jaeger-ui/packages/jaeger-ui/src/api/v3/client.ts` calls `/services`, `/operations` and `/trace-summaries` with `fetch` plus `response.json()` and Zod validation. Nothing in the UI calls `/api/v3/traces` yet; trace loading still uses the legacy API. |

## 4. Opting in: content negotiation on `Accept`

A client requests a streaming response by naming the streaming media type in the `Accept` request header. The gateway serves a stream when, and only when, `application/x-ndjson` is listed explicitly with a non-zero quality value. Its position and quality relative to other listed types are not compared, so `Accept: application/json, application/x-ndjson;q=0.1` streams: a client that names the streaming type at all has code to consume it, and a client that does not want it leaves it out or sets `q=0`, which is a simpler rule than ranking and produces the same result for every client that is not deliberately testing the edge. A missing `Accept`, a missing `Accept`, a wildcard such as `*/*` or `application/*`, `application/json`, or `application/x-ndjson;q=0` produces the buffered response exactly as today. Wildcards never select streaming, because a client that sends `*/*` is asking for whatever the server prefers, and the server prefers the format existing clients parse. The response carries the streaming media type in `Content-Type` so a client can confirm which form it received. Both representations of a streaming-capable endpoint carry `Vary: Accept` (alongside the `Accept-Encoding` value that compression adds in §8), so that a browser or intermediary cache never serves a stored NDJSON body to a client that asked for `application/json`, or the reverse.

```http
GET /api/v3/traces?query.serviceName=frontend HTTP/1.1
Accept: application/x-ndjson

HTTP/1.1 200 OK
Content-Type: application/x-ndjson
Vary: Accept
Transfer-Encoding: chunked
```

The alternatives were a deployment-wide feature gate and a query parameter. A gate flips the format for every client of a deployment at once, which is the breaking change G2 rules out unless every client has already been updated, and it gives a client no way to choose per request. A query parameter such as `?stream=true` adds a parameter the gRPC service does not have, so the HTTP surface would stop being a projection of the proto. `Accept` is standard HTTP, per request, and needs no new API vocabulary. The `Accept`-header approach was also the one point of the earlier ADR draft that met no objection on the issue.

## 5. Message framing: the core decision

Three framings can carry a sequence of JSON messages over one HTTP response.

- **NDJSON** (`application/x-ndjson`): each message is one JSON object serialized without embedded newlines, terminated by `\n`. The stream ends when the body ends. This is what grpc-gateway emits for server-streaming methods, and the `{"result": …}` / `{"error": …}` envelope the gateway inherited from grpc-gateway was designed for it.
- **SSE** (`text/event-stream`): each message is a `data:` field holding the JSON, terminated by a blank line, optionally with an `event:` name. The WHATWG specification defines no end-of-stream signal because its `EventSource` client is expected to reconnect whenever the connection drops, so APIs layer an application-level terminator on top (OpenAI's `data: [DONE]`, MCP's `event: end`).
- **gRPC-Web** (`application/grpc-web+json`): each message is prefixed by one flag byte and a 4-byte big-endian length; a final frame with the trailer flag carries the gRPC status. Clients need a binary frame parser.

The criteria come from the goals. All three preserve gRPC message boundaries, so that is not a differentiator.

| Criterion | NDJSON | SSE | gRPC-Web |
| --- | --- | --- | --- |
| Consumable with `fetch` + `JSON.parse`, or `net/http` + `json.Decoder`, with no library | 🟢 | 🟡¹ | 🔴² |
| Works with the `Accept`-header opt-in of §4 | 🟢 | 🟢³ | 🟢 |
| Transparent stream-level gzip (§8) | 🟢 | 🟢 | 🟡⁴ |
| Unambiguous end and error signalling | 🟢⁵ | 🟡⁶ | 🟢 |
| New npm or Go dependencies for the UI and tests | 🟢 none | 🟢 none⁷ | 🔴⁸ |
| Inspectable with `curl` and `jq` | 🟢 | 🟡 | 🔴 |

Legend: 🟢 meets the criterion · 🟡 meets it with a caveat · 🔴 does not meet it.

- ¹ A browser can read SSE from `fetch` through a `ReadableStream`, but the client must implement the field parser (`data:`, `event:`, `id:`, multi-line `data:` joining, comment lines) rather than splitting on `\n` and calling `JSON.parse`. Go has no SSE decoder in the standard library.
- ² A 5-byte binary header per message means byte-level parsing in the browser and in Go, and the JSON variant of gRPC-Web is served by few implementations, so tooling assumes protobuf.
- ³ Both a `fetch` client and the native `EventSource` API can negotiate: `EventSource` cannot set arbitrary request headers, but it sends `Accept: text/event-stream` on its own, which the gateway could recognise. `EventSource` is nonetheless not the client to design for, because it reconnects on end of stream (⁶) and is `GET`-only.
- ⁴ gRPC-Web carries its own per-message compression flag. HTTP-level gzip also works, but a client library that expects the flag may not expect both.
- ⁵ In-band `{"error": …}` and `{"end": true}` objects terminate every stream; §6.5 analyses truncation detection.
- ⁶ SSE has no standard end marker either, and the `EventSource` reconnect behaviour turns a normal end of stream into a retry loop unless the client closes on an application-defined terminal event; the terminal event is mandatory for SSE where it is a design choice for NDJSON.
- ⁷ The UI already depends on `@ag-ui/client`, which contains an SSE parser, but that package is shaped around AG-UI events and is not a general SSE library; hand-parsing would be needed anyway.
- ⁸ The `grpc-web` npm package plus generated client code for the API v3 proto, or a hand-written frame parser, in a bundle that is already the subject of size scrutiny.

**Decision.** NDJSON. It is the only option that is green on the consumability and dependency criteria, it is the framing the gateway's envelope was designed for, and end-of-stream signalling is settled by the terminal object of §6.5. The media type is `application/x-ndjson`, the de facto registration that tooling recognises; `application/jsonl` is still working its way through IANA registration.

## 6. The stream contract

### 6.1 Envelope

The buffered responses the gateway writes today have two shapes. The trace endpoints wrap the OTLP document in a `GRPCGatewayWrapper`, a proto message with a single `result` field, which grpc-gateway introduced so that a stream of results and an in-stream error could share one response schema. The summaries endpoint writes its `FindTraceSummariesResponse` as the top-level object with no wrapper:

```
GET /api/v3/traces/{id}      {"result":{"resourceSpans":[…]}}
GET /api/v3/traces           {"result":{"resourceSpans":[…]}}
GET /api/v3/trace-summaries  {"summaries":[…],"nextPageToken":"…"}
```

Errors on every endpoint use a third shape, `GRPCGatewayError`: `{"error":{"httpCode":404,"message":"…"}}`.

**Streaming lines are normalized to the wrapped shape for every RPC.** Each line is an object with exactly one of three discriminator keys, and no other top-level key that discriminates: `result`, holding one gRPC response message of the RPC in its usual JSON form; `error`, holding the `GRPCGatewayError` details; or `end`, marking the successful end of the stream (§6.5):

```
GetTrace, FindTraces  {"result":{"resourceSpans":[…]}}
FindTraceSummaries    {"result":{"summaries":[…],"nextPageToken":"…"}}
FindSpans             {"result":{"spans":{"resourceSpans":[…]},"nextPageToken":"…"}}
any RPC, on failure   {"error":{"httpCode":500,"message":"storage: connection reset"}}
any RPC, on success   {"end":true}
```

The single key per line is what lets a client switch on one key and tell a message from an in-stream error or the end of the stream, which an unwrapped message cannot do reliably, and one rule for every RPC means one decoder serves all of them.

The extra level under `result` for the paginated RPCs (`spans`, `summaries`) is the standard JSON mapping of their response messages, not something the gateway adds. `FindSpansResponse` has two fields, `spans` of type OTLP `TracesData` and `next_page_token`. `TracesData` is an OpenTelemetry message with a single `resource_spans` field, so a page cursor cannot be placed inside it, and any response that carries both spans and a cursor needs a message of its own around them. `GetTrace` and `FindTraces` have no cursor, so their message is `TracesData` itself and `resourceSpans` sits directly under `result`. Flattening the paginated shape on HTTP would make the HTTP JSON diverge from the gRPC message, require a hand-written marshaller for those endpoints, and leave the OpenAPI schema unable to be generated from the proto.

The buffered shapes are treated differently by endpoint:

- **`GetTrace`, `FindTraces`:** already wrapped. The per-line schema of the stream is the buffered schema, so a client's parser and Zod schema serve both modes.
- **`FindSpans`:** its HTTP handler is still under review in [#9613](https://github.com/jaegertracing/jaeger/pull/9613), so nothing has shipped and there is no reason for the new endpoint to differ from the trace endpoints. The PR currently writes the `FindSpansResponse` unwrapped; this RFC asks it to wrap the buffered response as `{"result":{"spans":…,"nextPageToken":"…"}}` before it merges, so the endpoint never ships an unwrapped form.
- **`FindTraceSummaries`:** shipped unwrapped, and changing it would break its clients, so its buffered response stays as it is. Only its streaming form is wrapped. This is the one asymmetry between modes, and it is documented as such in the OpenAPI document (§10).

The proto `GRPCGatewayWrapper` types its `result` as `TracesData`, so it can carry only the trace endpoints' payload. The gateway writes the envelope for the other RPCs at the JSON level, marshalling the response message with `jsonpb` and wrapping the bytes in `{"result":…}`; no new proto message is needed for that.

### 6.2 Granularity

One line corresponds to one gRPC stream message, whatever that message is for the RPC. The HTTP gateway invents no granularity of its own, and a client that understands the gRPC stream understands the NDJSON stream.

| RPC | One line is | Notes |
| --- | --- | --- |
| `GetTrace` | one `TracesData` | With `rawTraces=false` (the HTTP query parameter for the proto field `raw_traces`; default) the query service aggregates storage chunks, so each line is one whole, adjusted trace. With `rawTraces=true` storage chunks pass through and a large trace may span consecutive lines, as it does over gRPC. Archive-storage results follow primary-storage results. |
| `FindTraces` | one `TracesData` | Same rules as `GetTrace`. |
| `FindTraceSummaries` | one `FindTraceSummariesResponse` | `next_page_token` is set only on the last line, as in the gRPC stream. |
| `FindSpans` (RFC 0016) | one `FindSpansResponse` | Each line carries a `spans` `TracesData` holding one storage chunk; a trace's matching spans may span consecutive lines. `next_page_token` is set only on the last line. |
| `GetServices`, `GetOperations` | not applicable | Unary RPCs; the gateway ignores a streaming `Accept` for them and returns the buffered document. |

Enforcing "one complete trace per line" was rejected. It would require the gateway to re-aggregate what `rawTraces=true` deliberately leaves split, and a client that wants to build its model incrementally, which is the point of streaming, would be forced to parse a whole trace in one call.

### 6.3 Errors before the first line

Parameter errors, tenancy denials, and storage errors raised before any result has been produced return the same status codes and bodies they return today (`400`, `403`, `404`, `500`, with the `text/plain` JSON body that `http.Error` writes). No streaming contract has been established at that point, and changing those responses is out of scope.

### 6.4 Errors after the first line

Once the gateway has written the `200` status and the first line, the status code cannot change. A storage or context error surfaced by the iterator is written as a final `{"error": …}` line in place of the `{"end": true}` line, the gateway flushes it, and returns from the handler, which ends the body. The `httpCode` is the code `tryHandleError` would have chosen for the same error before streaming, so the mapping of errors to codes is defined once. A client treats an error line as the failure of the whole request; the lines already received are a partial result and the client decides whether to keep them. The gateway logs an in-stream error the way `tryHandleError` logs a `500` today.

A failed `Write` or `Flush` means the client has gone away, and so does an iterator error that wraps `context.Canceled` from the request context, which is usually how a disconnect first shows up because the query service observes the cancelled context before the next write fails. In both cases the handler stops pulling from the iterator, which ends the `range` and lets the storage backend release its cursor, and it writes nothing more; there is no one to send an error line to, and the disconnect is logged at debug level at most because the client caused it. Only errors from the storage path itself take the error-line route above.

### 6.5 End of stream and truncation

A failed stream already ends with a terminal object, the `{"error": …}` line of §6.4. The question is how a successful stream ends: with the end of the body alone, or with a terminal object of its own, so that a client can distinguish a complete stream from one cut short by a crashed server, a dropped connection, or an intermediary.

| Criterion | Transport end of body only | Terminal `{"end": true}` line |
| --- | --- | --- |
| Truncation detected on HTTP/1.1 | 🟢¹ | 🟢 |
| Truncation detected on HTTP/2 | 🟢² | 🟢 |
| Truncation detected through an HTTP/1.0 intermediary or a buffering proxy | 🔴³ | 🟢 |
| Second, independent signal when gzip is on | 🟢⁴ | 🟢 |
| One rule for how every stream ends | 🟡⁵ | 🟢⁶ |
| Body is plain NDJSON that generic tooling consumes unchanged | 🟢 | 🟡⁷ |
| Client logic | 🟢 read to EOF | 🟢⁸ |
| Room for end-of-stream metadata later | 🔴 | 🟢⁹ |

- ¹ HTTP/1.1 chunked transfer coding ends with a zero-length chunk. If the connection closes before it, Go's `http.Client` returns `io.ErrUnexpectedEOF` from `Body.Read`, and a browser's `fetch` body reader rejects with a network `TypeError`.
- ² An HTTP/2 stream ends with a DATA frame carrying `END_STREAM`. A connection loss or `RST_STREAM` before it is surfaced as an error by every client.
- ³ An intermediary that downgrades to HTTP/1.0 has no chunked coding and signals end by closing the connection, which is indistinguishable from truncation. A proxy that re-frames the body with a `Content-Length` it computed from a truncated upstream has the same effect.
- ⁴ A gzip stream ends with an 8-byte trailer. A truncated gzip body fails to decode in Go (`gzip.Reader` returns `io.ErrUnexpectedEOF`) and in browsers.
- ⁵ A stream ends either with an `error` object or with nothing, so the client has two end conditions to handle and the success case is the absence of a signal.
- ⁶ Every stream ends with exactly one terminal object, `error` or `end`. Reaching EOF without one is a truncation by definition, which is the same contract gRPC gives through its trailing status.
- ⁷ Every line still parses as JSON, but the last one has no `result`, so `jq '.result.resourceSpans'` prints a `null` for it and a script that concatenates results must skip it.
- ⁸ The client already switches on the `result` and `error` keys to assemble chunks (for example, concatenating `resourceSpans` across lines); `end` is a third case in the same switch, plus one check that EOF was preceded by a terminal object.
- ⁹ The `end` object can later carry fields such as a count of messages sent or a warning list without a new line type.

**Decision.** A successful stream ends with a final `{"end": true}` line, and a client treats EOF without a preceding `end` or `error` object as truncation. The transport-level signals in ¹, ², ⁴ still apply and are what most clients will hit first; the terminal object covers the intermediaries they do not, and it turns the stream contract into one rule: every well-formed stream ends with exactly one terminal object. The cost is one more case in a switch the client already has, and a `null` for tooling that reads only `result`.

### 6.6 Empty results and the status code

Both trace endpoints return `404 No traces found` today when the iterator yields nothing, because `returnTraces` serves them both. The two cases are not alike:

- **`GetTrace`** asks for one identified resource, and a trace that does not exist is the textbook `404`. The gRPC handler agrees and returns `NotFound`.
- **`FindTraces`** is a search, and a search that matches nothing is a successful search with an empty result, the same as `FindTraceSummaries` and `FindSpans`, which already return `200` with an empty list. The gRPC `FindTraces` handler agrees too: it ends an empty stream with `OK`, so the HTTP `404` is a divergence between the two transports of one RPC.

**Decision.** `FindTraces` returns `200` with an empty result in both modes: `{"result":{}}` buffered, and a `200` followed directly by `{"end": true}` when streaming. The buffered body is `{"result":{}}` rather than `{"result":{"resourceSpans":[]}}` because the OTLP JSON mapping omits empty repeated fields, which every OTLP JSON consumer already has to accept as "no elements". `GetTrace` keeps its `404`. Changing the buffered `FindTraces` response is a behaviour change for clients that treat `404` as "no matches", and it is accepted here because it aligns the HTTP response with the gRPC one and with the other two search endpoints, and because a client that follows the OTLP JSON mapping treats an absent `resourceSpans` as empty already.

The `404` for `GetTrace` still has to be decided before the status line is written, and a streaming handler cannot drain the iterator first the way `returnTraces` does today. It takes the same decision one step earlier, after the first pull from the iterator rather than after the last. A "chunk" here is one element of the slice the iterator yields, which is exactly what the gRPC handler sends as one message, whether or not it holds spans; the gateway does not filter elements the gRPC handler would send, so the two transports agree on whether a trace was found. The storage contract forbids empty chunks, and with the default `rawTraces=false` the query service drops any that slip through while aggregating, so an empty `{"result":{}}` line can arise only from a backend that violates the contract in raw mode:

- The first pull yields a chunk: write `200` and the streaming `Content-Type`, then that chunk and the rest of the stream.
- The iterator ends without a chunk: write the `404` exactly as today.
- The first pull yields an error: write the status that error maps to, as today.

A non-empty result pays nothing for this, and the same first-pull structure serves the search endpoints, where an empty iterator produces the `200` and the `end` line.

## 7. Transport: HTTP/1.1 and HTTP/2

The contract in §6 is defined on the body, not on the transport coding. The gateway writes each line to the `http.ResponseWriter` and calls `Flush()` on it through the `http.Flusher` interface; the standard library chooses the framing:

- Over HTTP/1.1, which is what a plaintext deployment speaks, the server omits `Content-Length` and sends the body with `Transfer-Encoding: chunked`, one chunk per flush, and the zero-length chunk when the handler returns.
- Over HTTP/2, which a TLS deployment negotiates by ALPN with an HTTP/2-capable client, there is no chunked coding; each flush becomes a DATA frame and the handler's return sets `END_STREAM`.

The handler never sets `Transfer-Encoding` itself, and the RFC requires nothing of HTTP/2 that the server does not do today. Two operational notes belong in the documentation of the feature:

- The gateway flushes after every message. A `rawTraces=true` stream of small storage chunks flushes often; this is the simplest correct policy, and a size- or time-based coalescing threshold is a tuning knob that can be added if measurements call for it.
- `write_timeout` on the query HTTP server, when an operator sets it, bounds the whole response and will cut a long stream. The default is unset. The documentation for streaming should say so, and the same applies to the SSE endpoint already in the server.

## 8. Compression

The issue asks how streaming interacts with compression. The query HTTP server compresses nothing today, so the question is which compression scheme, if any, the server should adopt so that streaming and compression coexist. The UI is the primary consumer and it must be able to decode with browser support alone.

| Criterion | Whole-stream gzip, flushed per message | Per-message compressed frames | No compression on streams |
| --- | --- | --- | --- |
| Browser decodes incrementally with no client code | 🟢¹ | 🔴² | 🟢 |
| Go client decodes with no client code | 🟢 | 🔴 | 🟢 |
| Bandwidth | 🟢³ | 🟡 | 🔴 |
| Also compresses the buffered responses and the rest of the API | 🟢 | 🔴 | 🔴 |
| Server implementation | 🟢⁴ | 🟡 | 🟢 |
| Format stays plain NDJSON on the wire after decoding | 🟢 | 🔴 | 🟢 |

- ¹ `fetch` decodes `Content-Encoding: gzip` transparently and delivers decoded bytes to the `ReadableStream` as they arrive, provided the server flushes the gzip writer so each message's compressed bytes are actually emitted. A gzip writer that is never flushed buffers internally and the client sees nothing until the end, which is the "compress the whole content, then chunk it" approach the issue discussion rejected.
- ² A compression flag inside each frame is the gRPC-Web design and requires the client to decompress each message itself.
- ³ Each `Flush()` on a gzip writer emits a sync block and costs a few bytes plus a small loss of compression ratio relative to one uninterrupted stream. The message-level payloads are large enough (spans with attributes) that the ratio stays high.
- ⁴ `github.com/gorilla/handlers`, already a direct dependency, provides `CompressHandler`, whose `compressResponseWriter.Flush` flushes the compressor and then the underlying `http.Flusher`, which is exactly the per-message behaviour required. It cannot be used as is: it honours whichever of `gzip` or `deflate` a client lists first, and for `deflate` it writes raw DEFLATE (`flate.NewWriter`) where HTTP's `deflate` coding means zlib-wrapped data, so a client that advertises `deflate, gzip` would receive a body it cannot decode. The gateway therefore wraps it in a thin adapter that reduces the request's `Accept-Encoding` to `gzip` or nothing before delegating.

**Decision.** Whole-stream gzip negotiated by `Accept-Encoding`, applied by `handlers.CompressHandler` behind the gzip-only adapter of ⁴ in the `jaegerquery` HTTP handler chain in `cmd/jaeger/internal/extension/jaegerquery/internal/server.go`, next to the recovery handler. That chain serves everything on the query port, so the handler compresses the API v3 responses, the legacy `/api/` endpoints, the UI assets, the SSE chat endpoint and the MCP streamable-HTTP endpoint alike; `CompressHandler` flushes the compressor on every `Flush()`, so the streaming endpoints keep streaming. `CompressHandler` also deletes `Accept-Encoding` from the request before delegating, which would stop the OTLP reverse proxy on the same port from passing the client's negotiation to its upstream, so the adapter leaves the proxied routes out of the compression chain. The client controls it: the handler compresses only when the request carries `Accept-Encoding: gzip`, and a client that wants plain bytes omits the header. No server-side option is added, because that would express the same decision in two places. The gateway's `Flush()` call after each line then flushes the compressor and the socket together. The `confighttp` `middlewares` extension point was considered and rejected because it requires an OpenTelemetry middleware extension and operator configuration, and the collector ships no compression middleware. The UI assets are served decompressed today (§3), so compressing them on the way out is a gain, not a double encoding.

## 9. Clients

### 9.1 Go

A Go consumer wraps `resp.Body` in a `json.Decoder` and calls `Decode` into a struct with `Result json.RawMessage`, `Error *GRPCGatewayErrorDetails` and `End bool` fields until it has seen a terminal object. The `result` bytes are then decoded with `ptrace.JSONUnmarshaler` (or, for the paginated RPCs, with `jsonpb` into the response message). `encoding/json` does not call the gogo `UnmarshalJSONPB` method that `jptrace.TracesData` implements, so decoding `result` straight into that type would silently drop the spans; the `json.RawMessage` step is what keeps the two libraries apart. An `io.EOF` or `io.ErrUnexpectedEOF` before a terminal object is a truncated stream. This decoder is the client side of the gateway's tests, which must consume the response as a client would rather than assert on substrings of the recorded body.

### 9.2 Jaeger UI

The UI reads the stream with `fetch` and `response.body.getReader()`. Each `read()` returns whatever bytes have arrived, which can end in the middle of a line or of a multi-byte UTF-8 character, so the reader decodes with `TextDecoder` in `stream: true` mode (which holds back a split character), appends the text to a carry-over buffer, splits the buffer on `\n`, parses every element but the last with `JSON.parse`, and keeps the last element as the new buffer because it may be an incomplete line. The server always writes whole lines, so partial lines exist only on the receiving side between reads; when the stream closes normally the buffer is empty, and a non-empty buffer at close is truncation. The reader validates a `result` with the existing `TracesDataSchema` from `src/api/v3/schemas.ts`, stops on `end`, and rejects on `error` or on a stream that closes before either. This is about twenty lines in `src/api/v3/client.ts` and adds no dependency. `EventSource` is not used, and `@ag-ui/client` stays confined to the AI assistant.

## 10. API specification

The API v3 OpenAPI document lives in `jaeger-idl` (`swagger/api_v3/query_service.openapi.yaml`), and the UI generates its Zod schemas from it. The streaming methods gain a second response media type, `application/x-ndjson`, whose per-line schema is a union of three branches: `{"result": <that method's response message>}`, `GRPCGatewayError`, and the `{"end": true}` terminator of §6.5; for the trace endpoints the first two are the existing `GRPCGatewayWrapper` and `GRPCGatewayError` messages. The `Vary: Accept` header of §4 is documented on both representations. The comment on `GRPCGatewayWrapper` in `proto/api_v3/query_service.proto`, which currently describes the future in the conditional, is rewritten to describe the convention as it stands. No message or RPC definition changes.

## 11. Backward compatibility

- A client that sends no `Accept`, `*/*`, or `application/json` receives byte-for-byte the response it receives today, with one exception: an empty `FindTraces` search returns `200` with `{"result":{}}` instead of `404` (§6.6).
- A client that sends `Accept: application/x-ndjson` to a unary endpoint receives the buffered `application/json` document; content negotiation for streaming applies only to server-streaming RPCs.
- Adding `Accept-Encoding`-negotiated gzip to the server changes nothing for clients that do not send `Accept-Encoding`, and every HTTP client library decodes gzip transparently for those that do.
- The gRPC service is untouched.

## 12. Considered alternatives

- **JSON array written incrementally** (PR #7555). Breaks existing clients and is not incrementally parseable without a custom tokenizer. Rejected.
- **Always stream, no opt-in.** Breaks every existing client. Rejected by G2.
- **Deployment feature gate as the opt-in.** Per-deployment rather than per-request, and still a breaking change for whichever clients have not moved. Rejected in favour of `Accept` (§4).
- **Trace-level granularity enforced by the gateway.** Undoes `rawTraces=true` and forces whole-trace parsing on the client. Rejected (§6.2).
- **Native `EventSource` as the browser client.** It negotiates with its own `Accept: text/event-stream`, but it is `GET`-only, reconnects on end of stream unless the client closes on an application-defined terminal event, and ties the API to SSE. Rejected; `fetch` with a `ReadableStream` is the browser model (§5, §9.2).
- **HTTP trailers for the final status.** Correct on HTTP/2, but `fetch` cannot read trailers in browsers and HTTP/1.1 trailers require the client to announce `TE: trailers`. Rejected in favour of the in-band terminal objects (§6.4, §6.5).
- **Compress the whole content, then apply chunked coding.** Suggested on the issue. The client can decode nothing until the last byte, which defeats streaming. Rejected (§8).

## 13. Implementation roadmap

Each milestone is one PR and is independently shippable.

- **M1 — Streaming `GetTrace` and `FindTraces` (jaeger).** `Accept: application/x-ndjson` negotiation in `http_gateway.go`; first-message pull before committing the status and `200` for an empty `FindTraces` (§6.6); one flushed line per gRPC message; terminal `end` or `error` line; buffered path otherwise unchanged. Tests include a Go NDJSON client (§9.1) that consumes the recorded stream, an in-stream error case, a truncation case (body closed before the terminal object), an empty-result case, and a `rawTraces=true` case where a trace spans lines.
- **M2 — Response compression on the query HTTP server (jaeger).** `handlers.CompressHandler` behind a gzip-only `Accept-Encoding` adapter (§8 ⁴) in the `cmd/jaeger/internal/extension/jaegerquery/internal/server.go` chain; a test that streams through gzip and shows the client decoding each line before the stream ends; a test that `Accept-Encoding: deflate, gzip` yields gzip and `Accept-Encoding: deflate` yields identity; a check that the SSE chat and MCP endpoints still stream through the compressor and that the OTLP reverse proxy still forwards the client's `Accept-Encoding`.
- **M3 — API specification (jaeger-idl).** The `application/x-ndjson` response for streaming methods in the OpenAPI document, and the rewritten `GRPCGatewayWrapper` comment.
- **M4 — Streaming `FindTraceSummaries` and `FindSpans` (jaeger).** Apply the §6 convention to the remaining server-streaming RPCs; the `FindSpans` HTTP route lands in [#9613](https://github.com/jaegertracing/jaeger/pull/9613).
- **M5 — Jaeger UI consumption (jaeger-ui).** An NDJSON reader in `src/api/v3/client.ts` and a `fetchTrace` on the v3 client that yields `TracesData` chunks; adoption by the trace page follows the UI's own v3 migration.

## 14. Open questions

- Whether the per-message flush should be measured against a coalescing threshold for `rawTraces=true` streams of small chunks before M1 merges, or left as a follow-up. The recommendation is to ship per-message flushing and measure in M1's review.
