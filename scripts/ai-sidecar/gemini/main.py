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


logger = logging.getLogger(__name__)

DEFAULT_MCP_URL = "http://127.0.0.1:16687/mcp"
DEFAULT_SIDECAR_PORT = 16688
DEFAULT_MCP_DISCOVERY_TIMEOUT_SEC = 15.0


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
    return parser.parse_args()


def parse_config() -> tuple[str, int, SidecarConfig]:
    args = parse_args()

    config = SidecarConfig(
        gemini_api_key=args.gemini_api_key,
        mcp_url=args.mcp_url,
        mcp_discovery_timeout_sec=args.mcp_discovery_timeout_sec,
    )
    config.validate()
    return args.host, args.port, config


async def main() -> None:
    host, port, config = parse_config()
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