# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import argparse
import os
from functools import partial

import websockets

from sidecar import DEFAULT_MCP_URL, JaegerSidecarAgent, SidecarConfig, handle_websocket


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run Jaeger Gemini ACP sidecar")
    parser.add_argument("--host", default="localhost", help="Host interface to bind")
    parser.add_argument("--port", type=int, default=9000, help="Port to listen on")
    parser.add_argument("--gemini-api-key", default=os.environ.get("GEMINI_API_KEY", ""), help="Gemini API key")
    parser.add_argument("--mcp-url", default=os.environ.get("JAEGER_MCP_URL", DEFAULT_MCP_URL), help="Jaeger MCP URL")
    return parser.parse_args()


def parse_config() -> tuple[str, int, SidecarConfig]:
    args = parse_args()
    if not args.gemini_api_key:
        raise RuntimeError("GEMINI_API_KEY must be provided via --gemini-api-key or environment variable")

    config = SidecarConfig(
        gemini_api_key=args.gemini_api_key,
        mcp_url=args.mcp_url,
    )
    return args.host, args.port, config


async def main():
    host, port, config = parse_config()
    async with websockets.serve(partial(handle_websocket, agent_factory=lambda: JaegerSidecarAgent(config)), host, port):
        print(f"Jaeger ACP Sidecar listening on ws://{host}:{port}")
        await asyncio.Future()


if __name__ == "__main__":
    asyncio.run(main())