# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Tests for the OpenRouter sidecar.

These lean on the design intent: most of them assert that the sidecar does NOT
do something (does not massage schemas, does not swallow a malformed tool call,
does not end a runaway turn quietly). Those are the properties that make the
transcripts usable as evaluation evidence, so they are the ones worth pinning.
"""

from __future__ import annotations

import json
from typing import Any

import pytest
from acp.schema import HttpMcpServer, HttpHeader

import sidecar as sidecar_mod
from sidecar import BASE_SYSTEM_PROMPT, MAX_ITERATIONS, JaegerOpenRouterAgent, SessionState
from transcript import Transcript



class FakeTool:
    def __init__(self, name: str, schema: dict[str, Any], description: str = "d"):
        self.name = name
        self.input_schema = schema
        self.description = description


class FakeMCP:
    """Stands in for JaegerMCPClient without touching the network."""

    def __init__(self, tools: list[FakeTool] | None = None, instructions: str | None = None):
        self._tools = tools or [FakeTool("get_trace", {"type": "object", "properties": {"id": {"type": "string"}}})]
        self._instructions = instructions
        self.url = "http://mcp.test/mcp"
        self.closed = False
        self.calls: list[tuple[str, dict[str, Any]]] = []
        self.fail_with: Exception | None = None

    @property
    def instructions(self) -> str | None:
        return self._instructions

    async def connect(self) -> None:
        return None

    async def close(self) -> None:
        self.closed = True

    def openai_tools(self) -> list[dict[str, Any]]:
        return [
            {
                "type": "function",
                "function": {"name": t.name, "description": t.description, "parameters": t.input_schema},
            }
            for t in self._tools
        ]

    async def call_tool(self, name: str, args: dict[str, Any]) -> str:
        self.calls.append((name, args))
        if self.fail_with is not None:
            raise self.fail_with
        return f"result-for-{name}"


class FakeConn:
    def __init__(self) -> None:
        self.updates: list[tuple[str, Any]] = []

    async def session_update(self, session_id: str, update: Any) -> None:
        self.updates.append((session_id, update))


def make_agent(**kwargs: Any) -> JaegerOpenRouterAgent:
    agent = JaegerOpenRouterAgent(  # pyright: ignore[reportAbstractUsage]
        model=kwargs.pop("model", "test/model"),
        api_key="secret-key",
        fallback_mcp_url=kwargs.pop("fallback_mcp_url", "http://fallback.test/mcp"),
        transcript=kwargs.pop("transcript", Transcript(None)),
    )
    agent.on_connect(FakeConn())  # pyright: ignore[reportArgumentType]
    return agent


def install_session(agent: JaegerOpenRouterAgent, mcp: FakeMCP, session_id: str = "sess-1") -> SessionState:
    state = SessionState(mcp)  # pyright: ignore[reportArgumentType]
    agent._sessions[session_id] = state
    return state


def completion(tool_calls: list[dict[str, Any]] | None = None, content: str = "") -> dict[str, Any]:
    message: dict[str, Any] = {"role": "assistant", "content": content}
    if tool_calls:
        message["tool_calls"] = tool_calls
    return {"choices": [{"message": message, "finish_reason": "stop"}]}


def tool_call(call_id: str, name: str, arguments: str) -> dict[str, Any]:
    return {"id": call_id, "type": "function", "function": {"name": name, "arguments": arguments}}


def stub_chat(agent: JaegerOpenRouterAgent, responses: list[dict[str, Any]]) -> list[list[dict[str, Any]]]:
    """Replace the OpenRouter call; records the messages sent each time."""
    seen: list[list[dict[str, Any]]] = []
    queue = list(responses)

    async def _chat(_client: Any, messages: list[dict[str, Any]], _tools: list[dict[str, Any]]) -> dict[str, Any]:
        seen.append([dict(m) for m in messages])
        return queue.pop(0) if queue else completion(content="done")

    agent._chat = _chat  # pyright: ignore[reportAttributeAccessIssue]
    return seen


# --- capabilities -----------------------------------------------------------


async def test_initialize_advertises_http_mcp_and_session_close() -> None:
    """The gateway gates its MCP announcement on mcp_capabilities.http.

    Drop this and session/new arrives with an empty mcpServers list, so the
    sidecar never learns the turn-scoped URL.
    """
    agent = make_agent()
    resp = await agent.initialize(sidecar_mod.PROTOCOL_VERSION)

    assert resp.agent_capabilities is not None
    assert resp.agent_capabilities.mcp_capabilities is not None
    assert resp.agent_capabilities.mcp_capabilities.http is True
    assert resp.agent_capabilities.session_capabilities is not None
    assert resp.agent_capabilities.session_capabilities.close is not None


async def test_initialize_rejects_mismatched_protocol_version() -> None:
    agent = make_agent()
    with pytest.raises(ValueError, match="Unsupported ACP protocol version"):
        await agent.initialize(sidecar_mod.PROTOCOL_VERSION + 1)


# --- MCP endpoint resolution ------------------------------------------------


async def test_new_session_prefers_gateway_announced_mcp_server() -> None:
    agent = make_agent()
    announced = HttpMcpServer(
        type="http",
        name="jaeger",
        url="http://gateway.test/api/ai/mcp/route-42/",
        headers=[HttpHeader(name="X-Turn", value="42")],
    )

    resp = await agent.new_session(cwd="/tmp", mcp_servers=[announced])
    state = agent._sessions[resp.session_id]

    assert state.mcp.url == "http://gateway.test/api/ai/mcp/route-42/"
    assert state.mcp._headers == {"X-Turn": "42"}


async def test_new_session_falls_back_when_nothing_announced() -> None:
    agent = make_agent(fallback_mcp_url="http://fallback.test/mcp")
    resp = await agent.new_session(cwd="/tmp", mcp_servers=[])
    assert agent._sessions[resp.session_id].mcp.url == "http://fallback.test/mcp"


async def test_close_session_closes_the_mcp_client() -> None:
    agent = make_agent()
    mcp = FakeMCP()
    install_session(agent, mcp)

    await agent.close_session("sess-1")

    assert mcp.closed is True
    assert "sess-1" not in agent._sessions


async def test_close_all_closes_every_live_session() -> None:
    agent = make_agent()
    first, second = FakeMCP(), FakeMCP()
    install_session(agent, first, "sess-1")
    install_session(agent, second, "sess-2")

    await agent.close_all()

    assert first.closed and second.closed
    assert agent._sessions == {}


# --- MCP instructions reaching the model ------------------------------------


async def test_server_instructions_are_relayed_into_the_system_prompt() -> None:
    """The regression this sidecar was built to fix.

    jaegermcp explains how to enter the skills catalog through the MCP
    initialize result's `instructions`. A sidecar that lists tools but drops
    that text leaves the model unaware a skills index exists at all.
    """
    agent = make_agent()
    state = install_session(agent, FakeMCP(instructions="Start at SKILL.md, then follow the wiki links."))

    await agent._start_session(state, "sess-1")

    system = state.messages[0]
    assert system["role"] == "system"
    assert "Start at SKILL.md, then follow the wiki links." in system["content"]
    assert BASE_SYSTEM_PROMPT in system["content"]


async def test_missing_server_instructions_leaves_the_base_prompt_intact() -> None:
    agent = make_agent()
    state = install_session(agent, FakeMCP(instructions=None))

    await agent._start_session(state, "sess-1")

    assert state.messages[0]["content"] == BASE_SYSTEM_PROMPT


async def test_tool_schemas_pass_through_unmodified() -> None:
    """No massaging: whatever jaegermcp publishes is what the model sees."""
    schema = {
        "type": "object",
        "properties": {"start": {"type": "string", "format": "date-time"}},
        "required": ["start"],
        "additionalProperties": False,
    }
    agent = make_agent()
    state = install_session(agent, FakeMCP(tools=[FakeTool("find_traces", schema)]))

    await agent._start_session(state, "sess-1")

    assert state.tools[0]["function"]["parameters"] == schema


async def test_session_is_started_only_once() -> None:
    agent = make_agent()
    state = install_session(agent, FakeMCP(instructions="once"))

    await agent._start_session(state, "sess-1")
    await agent._start_session(state, "sess-1")

    assert [m["role"] for m in state.messages] == ["system"]


# --- the loop ---------------------------------------------------------------


async def test_turn_without_tool_calls_returns_the_message_content() -> None:
    agent = make_agent()
    state = install_session(agent, FakeMCP())
    stub_chat(agent, [completion(content="no tools needed")])

    answer = await agent._run_loop("sess-1", state, "hello")

    assert answer == "no tools needed"


async def test_tool_results_are_appended_and_resent() -> None:
    """OpenRouter is stateless, so the full history goes back every round."""
    agent = make_agent()
    mcp = FakeMCP()
    state = install_session(agent, mcp)
    seen = stub_chat(
        agent,
        [
            completion(tool_calls=[tool_call("c1", "get_trace", '{"id": "abc"}')]),
            completion(content="final"),
        ],
    )

    answer = await agent._run_loop("sess-1", state, "investigate")

    assert answer == "final"
    assert mcp.calls == [("get_trace", {"id": "abc"})]
    second_request = seen[1]
    assert second_request[-1] == {
        "role": "tool",
        "tool_call_id": "c1",
        "content": "result-for-get_trace",
    }
    assert second_request[0]["role"] == "system"


async def test_tool_call_update_carries_raw_output() -> None:
    """Without raw_output the AG-UI stream shows the call but not its result."""
    agent = make_agent()
    conn: Any = agent._conn
    state = install_session(agent, FakeMCP())
    stub_chat(
        agent,
        [completion(tool_calls=[tool_call("c1", "get_trace", "{}")]), completion(content="ok")],
    )

    await agent._run_loop("sess-1", state, "go")

    completed = [u for _, u in conn.updates if getattr(u, "status", None) == "completed"]
    assert completed, "expected a completed tool_call update"
    assert completed[0].raw_output == {"content": "result-for-get_trace"}


async def test_failing_tool_is_reported_to_the_model_not_retried() -> None:
    agent = make_agent()
    mcp = FakeMCP()
    mcp.fail_with = RuntimeError("mcp exploded")
    state = install_session(agent, mcp)
    seen = stub_chat(
        agent,
        [completion(tool_calls=[tool_call("c1", "get_trace", "{}")]), completion(content="recovered")],
    )

    answer = await agent._run_loop("sess-1", state, "go")

    assert answer == "recovered"
    assert len(mcp.calls) == 1, "a retried tool call is a repair"
    assert json.loads(seen[1][-1]["content"]) == {"error": "mcp exploded"}


async def test_malformed_tool_call_arguments_raise() -> None:
    """Broken tool-call JSON must fail loudly rather than be patched to {}."""
    agent = make_agent()
    mcp = FakeMCP()
    state = install_session(agent, mcp)
    stub_chat(agent, [completion(tool_calls=[tool_call("c1", "get_trace", "{not json")])])

    with pytest.raises(json.JSONDecodeError):
        await agent._run_loop("sess-1", state, "go")

    assert mcp.calls == []


async def test_runaway_turn_raises_instead_of_truncating_silently() -> None:
    agent = make_agent()
    state = install_session(agent, FakeMCP())
    stub_chat(
        agent,
        [completion(tool_calls=[tool_call(f"c{i}", "get_trace", "{}")]) for i in range(MAX_ITERATIONS)],
    )

    with pytest.raises(RuntimeError, match="did not converge"):
        await agent._run_loop("sess-1", state, "go")


# --- prompt-level error surfacing -------------------------------------------


async def test_prompt_surfaces_errors_as_an_acp_update() -> None:
    """A turn that dies silently is indistinguishable from one that found nothing."""
    agent = make_agent()
    conn: Any = agent._conn
    state = install_session(agent, FakeMCP())
    stub_chat(agent, [completion(tool_calls=[tool_call("c1", "get_trace", "{oops")])])

    resp = await agent.prompt(session_id="sess-1", prompt=[])

    assert resp.stop_reason == "end_turn"
    texts = [str(u) for _, u in conn.updates]
    assert any("[Error:" in t for t in texts)
    assert state.messages, "session should still have been started"


async def test_prompt_creates_state_for_an_unknown_session() -> None:
    agent = make_agent()
    stub_chat(agent, [completion(content="hi")])

    await agent.prompt(session_id="never-opened", prompt=[])

    assert "never-opened" in agent._sessions


async def test_api_key_is_never_in_the_repr_or_logs(caplog: Any) -> None:
    agent = make_agent()
    state = install_session(agent, FakeMCP(instructions="x"))
    with caplog.at_level("DEBUG"):
        await agent._start_session(state, "sess-1")
    assert "secret-key" not in caplog.text


# --- transcript -------------------------------------------------------------


async def test_transcript_records_the_raw_tool_calls_block(tmp_path: Any) -> None:
    path = tmp_path / "run.jsonl"
    agent = make_agent(transcript=Transcript(str(path)))
    state = install_session(agent, FakeMCP())
    emitted = tool_call("c1", "get_trace", '{"id": "abc"}')
    stub_chat(agent, [completion(tool_calls=[emitted]), completion(content="final")])

    await agent.prompt(session_id="sess-1", prompt=[])

    events = [json.loads(line) for line in path.read_text().splitlines()]
    kinds = [e["event"] for e in events]
    assert kinds[0] == "prompt_received"
    assert "session_started" in kinds and "tool_executed" in kinds
    assert kinds[-1] == "turn_ended"

    first_completion = next(e for e in events if e["event"] == "completion")
    assert first_completion["tool_calls"] == [emitted], "recorded verbatim"

    started = next(e for e in events if e["event"] == "session_started")
    assert started["tool_names"] == ["get_trace"]


async def test_transcript_is_a_noop_when_unconfigured(tmp_path: Any) -> None:
    agent = make_agent(transcript=Transcript(None))
    state = install_session(agent, FakeMCP())
    stub_chat(agent, [completion(content="ok")])

    await agent.prompt(session_id="sess-1", prompt=[])

    assert list(tmp_path.iterdir()) == []


def test_transcript_from_env_treats_blank_as_disabled(monkeypatch: Any) -> None:
    monkeypatch.setenv("EVAL_TRANSCRIPT", "   ")
    assert Transcript.from_env().enabled is False
    monkeypatch.setenv("EVAL_TRANSCRIPT", "/tmp/t.jsonl")
    assert Transcript.from_env().enabled is True
    monkeypatch.delenv("EVAL_TRANSCRIPT")
    assert Transcript.from_env().enabled is False


def test_transcript_write_failure_does_not_raise(tmp_path: Any) -> None:
    unwritable = tmp_path / "nope" / "run.jsonl"
    Transcript(str(unwritable)).emit("sess-1", "completion", tool_calls=[])
