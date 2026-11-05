# Goose (ACP Agent)

Jaeger's AI gateway is bring-your-own-agent: it dials an
[ACP](https://agentclientprotocol.com/) agent over WebSocket and speaks the
protocol to it. No agent code lives here.

[Goose](https://github.com/aaif-goose/goose) is a reference implementation of
an ACP agent, so it works behind Jaeger's AI gateway as it ships. The steps to
configure it, point Jaeger at it, and verify the result are on the
website, on the
[AI Assistant page](https://www.jaegertracing.io/docs/latest/ai/assistant/).
This file keeps the prerequisites, the provider notes, and the authentication
note that the website does not carry.

## Prerequisites

- Jaeger 2.21 or later.
- The `goose` binary from [Goose releases](https://github.com/aaif-goose/goose/releases),
  v1.55.0 or later.
- Credentials for a model provider. The zero-cost route is
  [OpenRouter](https://openrouter.ai)'s free tier.

## Provider notes

Goose reads the provider settings from its **environment**, not from
`~/.config/goose/config.yaml`. Setting the API key only in the config file
leaves Goose reporting `Provider not set`.

The model **must support tool calling**, or it answers from prior knowledge and
never queries telemetry. To list OpenRouter's free tool-calling models:

```bash
curl -s https://openrouter.ai/api/v1/models \
  -H "Authorization: Bearer $OPENROUTER_API_KEY" |
  jq -r '.data[] | select(.supported_parameters | index("tools"))
         | select((.pricing.prompt|tonumber) == 0) | .id'
```

## Authentication

`goose serve` requires the value of `GOOSE_SERVER__SECRET_KEY` as an
`X-Secret-Key` header on the WebSocket upgrade, which Jaeger sends through
`ai.agent_headers`. Goose also accepts `--dangerously-unauthenticated`, which
disables that check. Do not use it with a Jaeger that can send headers.
