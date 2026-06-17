# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Standalone verification script for MCP resources (skills) endpoint.
# Connects to a running Jaeger MCP server and exercises resources/list
# and resources/read via raw Streamable HTTP — the same transport the
# sidecar uses.
#
# Usage:
#   uv run python verify_resources.py [--mcp-url URL]
#
# Default URL: http://localhost:16687/mcp

from __future__ import annotations

import argparse
import json
import sys
import requests


def _extract_json(text: str) -> dict | None:
    """Extract the last JSON-RPC message from an MCP Streamable HTTP response.

    Handles plain JSON and SSE. SSE events are blank-line-separated; each event
    may span multiple ``data:`` lines that must be concatenated before parsing.
    """
    last: dict | None = None
    for event in text.split("\n\n"):
        data_lines: list[str] = []
        for line in event.splitlines():
            line = line.strip()
            if line.startswith("data:"):
                data_lines.append(line[5:].strip())
        payload = "".join(data_lines)
        if payload.startswith("{"):
            try:
                last = json.loads(payload)
            except json.JSONDecodeError:
                continue
    if last is not None:
        return last
    try:
        return json.loads(text.strip())
    except json.JSONDecodeError:
        return None


def mcp_call(url: str, session_id: str | None, method: str, params: dict) -> tuple[dict, str | None]:
    body = {"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    if session_id:
        headers["Mcp-Session-Id"] = session_id
    resp = requests.post(url, json=body, headers=headers, timeout=10)
    resp.raise_for_status()
    session_id_out = resp.headers.get("Mcp-Session-Id", session_id)
    result = _extract_json(resp.text)
    if result is None:
        raise ValueError(f"could not parse MCP response for {method!r}: {resp.text[:200]}")
    return result, session_id_out


def send_initialized(url: str, session_id: str) -> None:
    """Send the required post-initialize notification before any other request."""
    body = {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
        "Mcp-Session-Id": session_id,
    }
    requests.post(url, json=body, headers=headers, timeout=10).raise_for_status()


def run(mcp_url: str) -> bool:
    print(f"Verifying MCP resources at {mcp_url}\n")
    ok = True

    # 1. Initialize session
    print("Step 1: initialize session")
    result, sid = mcp_call(mcp_url, None, "initialize", {
        "protocolVersion": "2025-03-26",
        "capabilities": {},
        "clientInfo": {"name": "verify-resources", "version": "1.0.0"},
    })
    if "error" in result:
        print(f"  FAIL initialize: {result['error']}")
        return False
    print(f"  OK  session_id={sid}")
    print(f"      server={result.get('result', {}).get('serverInfo', {})}")
    if sid:
        send_initialized(mcp_url, sid)

    # 2. resources/list
    print("\nStep 2: resources/list")
    result, _ = mcp_call(mcp_url, sid, "resources/list", {})
    if "error" in result:
        print(f"  FAIL resources/list: {result['error']}")
        ok = False
    else:
        resources = result.get("result", {}).get("resources", [])
        print(f"  OK  {len(resources)} resource(s) returned")
        for r in resources:
            print(f"      {r['uri']!r}  [{r.get('mimeType', '?')}]  — {r.get('description', '')[:60]}")
        expected = {"skill://skills-index", "skill://greet-user", "skill://echo-message"}
        got = {r["uri"] for r in resources}
        missing = expected - got
        if missing:
            print(f"  FAIL missing expected URIs: {missing}")
            ok = False
        else:
            print("  OK  all expected skill:// URIs present")

    # 3. resources/read for each skill
    print("\nStep 3: resources/read for each skill")
    for uri in ["skill://skills-index", "skill://greet-user", "skill://echo-message"]:
        result, _ = mcp_call(mcp_url, sid, "resources/read", {"uri": uri})
        if "error" in result:
            print(f"  FAIL {uri}: {result['error']}")
            ok = False
        else:
            contents = result.get("result", {}).get("contents", [])
            if not contents:
                print(f"  FAIL {uri}: empty contents")
                ok = False
            else:
                body = contents[0].get("text", "")
                first_line = body.split("\n")[0]
                print(f"  OK  {uri}: {len(body)} bytes, starts with {first_line!r}")

    # 4. resources/read unknown URI → expect error
    print("\nStep 4: resources/read unknown skill → expect error")
    result, _ = mcp_call(mcp_url, sid, "resources/read", {"uri": "skill://nonexistent"})
    if "error" in result:
        print(f"  OK  got expected error: {result['error'].get('message', result['error'])}")
    else:
        print("  FAIL expected an error for unknown skill URI, but got a result")
        ok = False

    # 5. Clean up session
    if sid:
        requests.delete(mcp_url, headers={"Mcp-Session-Id": sid}, timeout=5)

    print(f"\n{'PASS' if ok else 'FAIL'}: MCP resources verification {'passed' if ok else 'FAILED'}")
    return ok


def main() -> None:
    parser = argparse.ArgumentParser(description="Verify MCP skill resources endpoint")
    parser.add_argument("--mcp-url", default="http://localhost:16687/mcp", help="MCP server URL")
    args = parser.parse_args()
    sys.exit(0 if run(args.mcp_url) else 1)


if __name__ == "__main__":
    main()
