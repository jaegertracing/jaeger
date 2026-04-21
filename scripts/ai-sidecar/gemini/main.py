# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import argparse
import logging
import os
from functools import partial

import websockets

from sidecar import JaegerSidecarAgent, handle_websocket
from sidecar_config import SidecarConfig
from tracing import init_tracing


logger = logging.getLogger(__name__)

DEFAULT_MCP_URL = "http://127.0.0.1:16687/mcp"
DEFAULT_SIDECAR_PORT = 16688
DEFAULT_MCP_DISCOVERY_TIMEOUT_SEC = 15.0
DEFAULT_OTLP_ENDPOINT = "http://localhost:4317"
DEFAULT_OTLP_INSECURE = True


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run Jaeger Gemini ACP sidecar")
    parser.add_argument("--host", default="localhost", help="Host interface to bind")
    parser.add_argument("--port", type=int, default=DEFAULT_SIDECAR_PORT, help="Port to listen on")
    parser.add_argument("--gemini-api-key", default=os.environ.get("GEMINI_API_KEY", ""), help="Gemini API key")
    parser.add_argument("--mcp-url", default=os.environ.get("JAEGER_MCP_URL", DEFAULT_MCP_URL), help="Jaeger MCP URL")
    parser.add_argument(
        "--mcp-discovery-timeout-sec",
        type=float,
        default=float(
            os.environ.get(
                "JAEGER_MCP_DISCOVERY_TIMEOUT_SEC",
                str(DEFAULT_MCP_DISCOVERY_TIMEOUT_SEC),
            )
        ),
        help="Timeout (seconds) for single MCP tool discovery attempt",
    )
    parser.add_argument(
        "--otlp-endpoint",
        default=os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT", DEFAULT_OTLP_ENDPOINT),
        help="OTLP receiver endpoint for trace export",
    )
    parser.add_argument(
        "--otlp-insecure",
        type=lambda v: v.lower() in ("true", "1", "yes"),
        default=os.environ.get("OTEL_EXPORTER_OTLP_INSECURE", str(DEFAULT_OTLP_INSECURE)).lower() in ("true", "1", "yes"),
        help="Skip TLS for OTLP export (default: true)",
    )
    return parser.parse_args()


def parse_config() -> tuple[str, int, SidecarConfig]:
    args = parse_args()

    config = SidecarConfig(
        gemini_api_key=args.gemini_api_key,
        mcp_url=args.mcp_url,
        mcp_discovery_timeout_sec=args.mcp_discovery_timeout_sec,
        otlp_endpoint=args.otlp_endpoint,
        otlp_insecure=args.otlp_insecure,
    )
    config.validate()
    return args.host, args.port, config


async def main() -> None:
    host, port, config = parse_config()
    init_tracing(endpoint=config.otlp_endpoint, insecure=config.otlp_insecure)
    # The lambda below is an agent factory, not a single shared instance.
    # Every new WebSocket connection invokes it to create a fresh JaegerSidecarAgent.
    # Each connection gets its own agent instance, so active connections can process prompts concurrently.
    async with websockets.serve(
        partial(handle_websocket, agent_factory=lambda: JaegerSidecarAgent(config)),  # pyright: ignore[reportAbstractUsage]
        host,
        port,
    ):
        logger.info("Jaeger ACP Sidecar listening on ws://%s:%s", host, port)
        await asyncio.Future()


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    asyncio.run(main())
