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

# Jaeger serves the MCP tools in-process on the query port when
# `jaeger_query.ai.enable_mcp` is set; the standalone jaeger_mcp extension that
# used to listen on :16687 is retired.
DEFAULT_MCP_URL = "http://localhost:16686/api/ai/mcp/"
DEFAULT_SIDECAR_PORT = 16688
DEFAULT_MCP_DISCOVERY_TIMEOUT_SEC = 15.0
DEFAULT_OTLP_ENDPOINT = "http://localhost:4317"
DEFAULT_OTLP_INSECURE = True
DEFAULT_OLLAMA_URL = "http://localhost:11434"
# The model must support tool calling, or the agent cannot query MCP for the
# telemetry it is supposed to reason about.
DEFAULT_MODEL = "qwen3:8b"
# A local model is slower than a hosted API, and the first request also pays to
# load the weights into memory, so the default is generous rather than snappy.
DEFAULT_OLLAMA_TIMEOUT_SEC = 300.0


def parse_bool(value: str) -> bool:
    return value.lower() in ("true", "1", "yes")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run Jaeger Ollama ACP sidecar")
    parser.add_argument("--host", default="localhost", help="Host interface to bind")
    parser.add_argument("--port", type=int, default=DEFAULT_SIDECAR_PORT, help="Port to listen on")
    parser.add_argument(
        "--ollama-url",
        default=os.environ.get("JAEGER_AI_OLLAMA_URL", DEFAULT_OLLAMA_URL),
        help="Base URL of the Ollama server",
    )
    parser.add_argument(
        "--model",
        default=os.environ.get("JAEGER_AI_MODEL", DEFAULT_MODEL),
        help="Ollama model to run; must support tool calling",
    )
    parser.add_argument(
        "--ollama-timeout-sec",
        type=float,
        default=float(
            os.environ.get(
                "JAEGER_AI_OLLAMA_TIMEOUT_SEC",
                str(DEFAULT_OLLAMA_TIMEOUT_SEC),
            )
        ),
        help="Timeout (seconds) for a single Ollama chat request",
    )
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
        action=argparse.BooleanOptionalAction,
        default=parse_bool(os.environ.get("OTEL_EXPORTER_OTLP_INSECURE", str(DEFAULT_OTLP_INSECURE))),
        help="Skip TLS for OTLP export; use --otlp-insecure or --no-otlp-insecure",
    )
    return parser.parse_args()


def parse_config() -> tuple[str, int, SidecarConfig]:
    args = parse_args()

    config = SidecarConfig(
        ollama_url=args.ollama_url,
        model=args.model,
        ollama_timeout_sec=args.ollama_timeout_sec,
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
        logger.info(
            "Jaeger ACP Sidecar listening on ws://%s:%s (model=%s via %s)",
            host,
            port,
            config.model,
            config.ollama_url,
        )
        await asyncio.Future()


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    asyncio.run(main())
