# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from dataclasses import dataclass


@dataclass(frozen=True)
class SidecarConfig:
    gemini_api_key: str
    mcp_url: str
    mcp_discovery_timeout_sec: float

    def validate(self) -> None:
        if not self.gemini_api_key:
            raise RuntimeError(
                "GEMINI_API_KEY must be provided via --gemini-api-key or environment variable"
            )
        if not self.mcp_url:
            raise RuntimeError("JAEGER_MCP_URL must be provided via --mcp-url or environment variable")
        if self.mcp_discovery_timeout_sec <= 0:
            raise RuntimeError("MCP discovery timeout must be > 0 seconds")
