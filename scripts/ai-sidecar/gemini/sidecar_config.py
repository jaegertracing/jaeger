# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from dataclasses import dataclass


@dataclass(frozen=True)
class SidecarConfig:
    gemini_api_key: str
    mcp_url: str
    mcp_discovery_timeout_sec: float
    otlp_endpoint: str
    otlp_insecure: bool

    def validate(self) -> None:
        if not self.gemini_api_key or not self.gemini_api_key.strip():
            raise RuntimeError(
                "GEMINI_API_KEY must be provided via --gemini-api-key or environment variable"
            )
        if not self.mcp_url or not self.mcp_url.strip():
            raise RuntimeError("JAEGER_MCP_URL must be provided via --mcp-url or environment variable")
        if not self.mcp_url.strip().startswith(("http://", "https://")):
            raise RuntimeError("JAEGER_MCP_URL must start with http:// or https://")
        if self.mcp_discovery_timeout_sec <= 0:
            raise RuntimeError("MCP discovery timeout must be > 0 seconds")
        if not self.otlp_endpoint or not self.otlp_endpoint.strip():
            raise RuntimeError(
                "OTEL_EXPORTER_OTLP_ENDPOINT must be provided via --otlp-endpoint or environment variable"
            )

