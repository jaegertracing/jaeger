# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import pytest
from unittest.mock import AsyncMock, MagicMock

from google.genai import types

from sidecar import JaegerSidecarAgent
from sidecar_config import SidecarConfig


async def _test_agentic_loop_prepends_mcp_instructions():
    config = SidecarConfig(
        gemini_api_key="fake-key",
        mcp_url="http://fake-mcp",
        mcp_discovery_timeout_sec=5.0,
        otlp_endpoint="http://fake-otlp",
        otlp_insecure=True,
    )
    agent = JaegerSidecarAgent(config)  # type: ignore
    
    # Stub MCP tools and instructions
    agent._mcp.get_gemini_tools = AsyncMock(return_value=[])
    agent._mcp.instructions = "custom mcp instructions"
    
    # Mock Gemini client chat
    mock_chat = MagicMock()
    # Mock send_message to return a response with no function calls so the loop terminates
    mock_response = MagicMock()
    mock_response.function_calls = []
    mock_response.text = "Final answer"
    mock_chat.send_message.return_value = mock_response
    
    agent._gemini = MagicMock()
    agent._gemini.chats.create.return_value = mock_chat
    
    # Call the agentic loop
    result = await agent._run_agentic_gemini_loop("session-1", "user prompt")
    
    assert result == "Final answer"
    
    # Assert chats.create was called with the prepended instructions
    agent._gemini.chats.create.assert_called_once()
    call_kwargs = agent._gemini.chats.create.call_args.kwargs
    config_arg = call_kwargs.get("config")
    
    assert config_arg is not None
    assert isinstance(config_arg, types.GenerateContentConfig)
    
    system_instruction = config_arg.system_instruction
    assert system_instruction is not None
    assert str(system_instruction).startswith("custom mcp instructions\n\nYou are Jaeger AI")

def test_agentic_loop_prepends_mcp_instructions():
    import asyncio
    asyncio.run(_test_agentic_loop_prepends_mcp_instructions())
