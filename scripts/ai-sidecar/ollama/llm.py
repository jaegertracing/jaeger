# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Ollama chat client for the sidecar's agentic loop.

This is the piece that differs from the Gemini reference sidecar. Everything
else — the WebSocket transport, the ACP handlers, the `_meta` parsing, the MCP
bridge, the contextual-tool dispatch — behaves the same way.

The model runs on the operator's machine, so no API key is required and no
prompt or telemetry leaves the host.
"""

import logging
from dataclasses import dataclass
from typing import Any

import httpx
from ollama import AsyncClient
from ollama._types import Message, ResponseError

from sidecar_helpers import _to_tool_text

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class ToolCall:
    """A tool invocation requested by the model."""

    name: str
    # Whatever the model emitted for the arguments. Deliberately not coerced
    # to a dict: _validate_function_call reports a malformed payload with the
    # tool name and call id attached, which beats silently substituting {}.
    args: Any


@dataclass(frozen=True)
class ToolResult:
    call: ToolCall
    output: Any


@dataclass(frozen=True)
class ModelTurn:
    """One model response: either a final answer or a batch of tool calls."""

    text: str
    tool_calls: list[ToolCall]


class OllamaChat:
    """A single multi-turn conversation with a local Ollama model.

    Ollama is stateless per request — the whole transcript is replayed on every
    call — so this object owns the message history. Assistant turns are
    appended verbatim, tool_calls included, because the model has to see its
    own tool requests next to the results it gets back.
    """

    def __init__(
        self,
        client: AsyncClient,
        model: str,
        system_instruction: str,
        tools: list[dict[str, Any]],
    ) -> None:
        self._client = client
        self._model = model
        self._tools = tools
        self._messages: list[Message | dict[str, Any]] = []
        if system_instruction:
            self._messages.append({"role": "system", "content": system_instruction})

    async def send_user_message(self, text: str) -> ModelTurn:
        self._messages.append({"role": "user", "content": text})
        return await self._advance()

    async def send_tool_results(self, results: list[ToolResult]) -> ModelTurn:
        for result in results:
            # tool_name is what lets a model line a result up with the call it
            # made when it requested several tools in one turn.
            self._messages.append(
                {
                    "role": "tool",
                    "tool_name": result.call.name,
                    "content": _to_tool_text(result.output),
                }
            )
        return await self._advance()

    async def _advance(self) -> ModelTurn:
        try:
            response = await self._client.chat(
                model=self._model,
                messages=self._messages,
                tools=self._tools or None,
                stream=False,
            )
        except (httpx.ConnectError, ConnectionError) as exc:
            raise RuntimeError(
                f"Unable to reach Ollama. Start it with `ollama serve`, or point "
                f"--ollama-url at the host running it. ({exc})"
            ) from exc
        except httpx.ReadTimeout as exc:
            raise RuntimeError(
                "Ollama did not respond in time. A local model can be slow on first load — "
                "raise --ollama-timeout-sec, or use a smaller model."
            ) from exc
        except ResponseError as exc:
            # Ollama's own message is the useful part: a model that hasn't been
            # pulled reports `model "x" not found, try pulling it first`.
            raise RuntimeError(f"Ollama rejected the request for model '{self._model}': {exc}") from exc

        message = response.message
        self._messages.append(message)
        return _to_turn(message)


def _to_turn(message: Message) -> ModelTurn:
    calls = [
        ToolCall(name=call.function.name, args=call.function.arguments)
        for call in (message.tool_calls or [])
    ]
    # `thinking` is deliberately dropped: reasoning models put their scratchpad
    # there and the answer in `content`, and only the answer belongs in chat.
    return ModelTurn(text=message.content or "", tool_calls=calls)


def build_ollama_client(base_url: str, timeout_sec: float) -> AsyncClient:
    return AsyncClient(host=base_url, timeout=timeout_sec)
