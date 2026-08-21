# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Entrypoint for the OpenRouter-backed Jaeger ACP sidecar.

Configuration is environment-first so the model can be swapped without touching
code or flags — that is the whole point of this arm: OPENROUTER_MODEL is the
independent variable in a cross-model skill evaluation.
"""

import argparse
import asyncio
import logging
import os
import signal
import sys
from functools import partial
from pathlib import Path

# scripts/ai-sidecar holds the `shared` package that every sidecar imports. This
# project runs as a directory of scripts rather than an installed package, so the
# parent directory has to be put on the path before `shared` resolves.
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import websockets

from sidecar import JaegerOpenRouterAgent, handle_websocket
from transcript import Transcript

logger = logging.getLogger(__name__)

DEFAULT_HOST = "localhost"
# Matches the Gemini sidecar and the gateway's DefaultAIAgentURL
# (ws://localhost:16688), so either sidecar drops into the same slot.
DEFAULT_PORT = 16688
# Only a fallback. In a gateway deployment the MCP URL arrives per session in
# session/new; see JaegerOpenRouterAgent._resolve_mcp.
DEFAULT_MCP_URL = "http://localhost:16686/mcp"
DEFAULT_MCP_TIMEOUT_SEC = 30.0
DEFAULT_MODEL_TIMEOUT_SEC = 120.0


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Jaeger ACP sidecar backed by OpenRouter")
    p.add_argument("--host", default=os.environ.get("SIDECAR_HOST", DEFAULT_HOST))
    p.add_argument("--port", type=int, default=int(os.environ.get("SIDECAR_PORT", DEFAULT_PORT)))
    p.add_argument("--model", default=os.environ.get("OPENROUTER_MODEL", ""))
    p.add_argument("--mcp-url", default=os.environ.get("JAEGER_MCP_URL", DEFAULT_MCP_URL))
    p.add_argument(
        "--mcp-timeout-sec",
        type=float,
        default=float(os.environ.get("JAEGER_MCP_TIMEOUT_SEC", DEFAULT_MCP_TIMEOUT_SEC)),
    )
    p.add_argument(
        "--model-timeout-sec",
        type=float,
        default=float(os.environ.get("OPENROUTER_TIMEOUT_SEC", DEFAULT_MODEL_TIMEOUT_SEC)),
    )
    return p.parse_args()


def load_api_key() -> str:
    """Read the API key from the environment only.

    Never a flag: a key on the command line lands in shell history and in every
    `ps` listing on the box. It is also never logged and never interpolated into
    an error message.
    """
    key = os.environ.get("OPENROUTER_API_KEY", "").strip()
    if not key:
        raise SystemExit("OPENROUTER_API_KEY is not set. Export it and retry.")
    return key


async def main() -> None:
    args = parse_args()
    api_key = load_api_key()
    if not args.model:
        raise SystemExit("OPENROUTER_MODEL is not set (or pass --model), e.g. google/gemini-2.5-flash")

    transcript = Transcript.from_env()

    # Every WebSocket connection gets its own agent so concurrent gateway
    # connections cannot see each other's sessions. The live set is tracked only
    # so shutdown can close their MCP clients.
    live: set[JaegerOpenRouterAgent] = set()

    def build_agent() -> JaegerOpenRouterAgent:
        # The ACP Agent base declares optional RPCs (session/set_mode, config
        # options, …) that this sidecar deliberately does not answer; the runtime
        # replies "method not found" for them, which is the correct behaviour for
        # an agent that does not offer the feature.
        agent = JaegerOpenRouterAgent(  # pyright: ignore[reportAbstractUsage]
            model=args.model,
            api_key=api_key,
            fallback_mcp_url=args.mcp_url,
            transcript=transcript,
            mcp_timeout_sec=args.mcp_timeout_sec,
            model_timeout_sec=args.model_timeout_sec,
        )
        live.add(agent)
        return agent

    stop = asyncio.get_running_loop().create_future()

    def request_stop(signame: str) -> None:
        logger.info("Received %s, shutting down", signame)
        if not stop.done():
            stop.set_result(None)

    loop = asyncio.get_running_loop()
    for signame in ("SIGINT", "SIGTERM"):
        loop.add_signal_handler(getattr(signal, signame), partial(request_stop, signame))

    handler = partial(handle_websocket, agent_factory=build_agent)
    async with websockets.serve(handler, args.host, args.port):
        logger.info(
            "Jaeger OpenRouter sidecar on ws://%s:%d (model=%s, fallback_mcp=%s, transcript=%s)",
            args.host,
            args.port,
            args.model,
            args.mcp_url,
            "on" if transcript.enabled else "off",
        )
        await stop

    for agent in list(live):
        await agent.close_all()
    logger.info("Shutdown complete")


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    asyncio.run(main())
