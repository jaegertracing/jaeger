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
    # Streamable HTTP may return SSE or JSON; extract the last JSON object.
    text = resp.text.strip()
    session_id_out = resp.headers.get("Mcp-Session-Id", session_id)
    for line in reversed(text.splitlines()):
        line = line.strip()
        if line.startswith("data:"):
            line = line[5:].strip()
        if line.startswith("{"):
            return json.loads(line), session_id_out
    return json.loads(text), session_id_out


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
