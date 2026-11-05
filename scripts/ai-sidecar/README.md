# AI sidecar

Jaeger's AI Assistant is bring-your-own-agent. The `jaeger_query` extension
dials an [ACP](https://agentclientprotocol.com/) agent that the operator runs,
and Jaeger ships no agent and no model provider of its own. This directory is
where a launcher or container recipe for a reference agent would live; it holds
none today.

Running an agent, with Goose as the worked example, is documented on the
website on the
[AI Assistant page](https://www.jaegertracing.io/docs/latest/ai/assistant/).
The gateway that talks to the agent is
[`jaegerai`](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md).
