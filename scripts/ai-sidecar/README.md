# AI sidecar

Jaeger's AI Assistant is bring-your-own-agent. The `jaeger_query` extension
dials an [ACP](https://agentclientprotocol.com/) agent that the operator runs,
and Jaeger ships no agent and no model provider of its own. This directory holds
the recipes for running a reference agent next to Jaeger, one subdirectory per
agent; [`goose/`](./goose/README.md) is the one for Goose.

Configuring the gateway and running an agent are documented on the website on
the [AI Assistant page](https://www.jaegertracing.io/docs/latest/ai/assistant/).
The gateway that talks to the agent is
[`jaegerai`](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md).
