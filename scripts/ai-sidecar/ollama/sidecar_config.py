# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from dataclasses import dataclass


@dataclass(frozen=True)
class SidecarConfig:
    ollama_url: str
    model: str
    ollama_timeout_sec: float
    mcp_url: str
    mcp_discovery_timeout_sec: float
    otlp_endpoint: str
    otlp_insecure: bool

    def validate(self) -> None:
        # No API key to check — the model runs locally. What has to be true is
        # that we know where it lives and which model to ask for.
        if not self.ollama_url:
            raise RuntimeError("Ollama URL must be provided via --ollama-url or JAEGER_AI_OLLAMA_URL")
        if not self.model:
            raise RuntimeError("Model name must be provided via --model or JAEGER_AI_MODEL")
        if self.ollama_timeout_sec <= 0:
            raise RuntimeError("Ollama request timeout must be > 0 seconds")
        if not self.otlp_endpoint:
            raise RuntimeError(
                "OTEL_EXPORTER_OTLP_ENDPOINT must be provided via --otlp-endpoint or environment variable"
            )
        if not self.mcp_url:
            raise RuntimeError("JAEGER_MCP_URL must be provided via --mcp-url or environment variable")
        if self.mcp_discovery_timeout_sec <= 0:
            raise RuntimeError("MCP discovery timeout must be > 0 seconds")
