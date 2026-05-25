// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// jaeger-ws-bridge: a WS→stdio bridge that lets Jaeger's AI gateway use
// claude-agent-acp as its ACP sidecar.
//
// The gateway dials ws://<this-host>:16688 by default and speaks ACP over
// newline-delimited JSON-RPC text frames. claude-agent-acp speaks the same
// JSON-RPC dialect but only over stdio. For each incoming WS connection we
// spawn a fresh claude-agent-acp subprocess and shuttle frames between the
// WS and the child's stdin/stdout.
//
// Per-connection subprocess is deliberate:
//   - Crash isolation: a runaway tool call kills only that one chat.
//   - Lifecycle parity: the gateway already opens one ACP session per chat
//     HTTP request and never reuses session ids, so a fresh agent per
//     connection matches the existing contract.
//   - Loose coupling: the bridge does not import claude-agent-acp's
//     internal APIs, so SDK refactors don't break it as long as the CLI
//     entry stays runnable.

import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { WebSocket, WebSocketServer } from "ws";

const require = createRequire(import.meta.url);

// Resolve the agent's CLI entry from its package.json so the bridge keeps
// working when claude-agent-acp's dist layout changes between releases.
function resolveAgentEntry() {
  const pkgJsonPath =
    require.resolve("@agentclientprotocol/claude-agent-acp/package.json");
  const pkg = require("@agentclientprotocol/claude-agent-acp/package.json");
  const binEntry =
    typeof pkg.bin === "string" ? pkg.bin : pkg.bin?.["claude-agent-acp"];
  if (!binEntry) {
    throw new Error(
      "claude-agent-acp package.json does not declare a runnable bin entry",
    );
  }
  return resolve(dirname(pkgJsonPath), binEntry);
}

const AGENT_ENTRY = resolveAgentEntry();
const HOST = process.env.HOST ?? "127.0.0.1";
const PORT = Number(process.env.PORT ?? 16688);

const wss = new WebSocketServer({ host: HOST, port: PORT });
console.error(`[bridge] listening on ws://${HOST}:${PORT}`);
console.error(`[bridge] agent entry: ${AGENT_ENTRY}`);

// Set DEBUG_BRIDGE=1 to log every JSON-RPC message in both directions.
// Off by default to keep production logs uncluttered.
const DEBUG = process.env.DEBUG_BRIDGE === "1";

wss.on("connection", (ws) => {
  // stdio: ["pipe", "pipe", "pipe"] — capture stderr so we can prefix it
  // with the child PID and surface any agent startup errors alongside our
  // own bridge logs. Inheriting stderr instead would mix unprefixed agent
  // output with bridge output and make multi-connection sessions
  // impossible to disambiguate.
  const agent = spawn(process.execPath, [AGENT_ENTRY], {
    stdio: ["pipe", "pipe", "pipe"],
    env: process.env,
  });
  const childPid = agent.pid;
  console.error(`[bridge:${childPid}] connection opened`);

  // Tee agent stderr to the bridge's stderr with a per-child prefix.
  const stderrLines = createInterface({ input: agent.stderr });
  stderrLines.on("line", (line) => {
    console.error(`[agent:${childPid}] ${line}`);
  });

  // ACP framing is one JSON-RPC message per newline on stdio. The gateway
  // side does line-based parsing on top of the WS stream
  // (jaegerai/ws_adapter.go's Read just concatenates frame payloads
  // without injecting newlines between them), so we MUST keep the `\n`
  // terminator on each outbound WS message — otherwise the gateway's
  // bufio reader concatenates JSON-RPC bodies and never produces a
  // complete line, leaving acp.SendRequest hung forever waiting for a
  // response that already arrived. createInterface strips the newline,
  // so we re-append it before sending. The Gemini sidecar relies on the
  // same invariant (ws_commands.py uses readline()).
  const lines = createInterface({ input: agent.stdout });
  lines.on("line", (line) => {
    if (line.length === 0) return;
    if (DEBUG) console.error(`[bridge:${childPid}] →ws: ${line}`);
    if (ws.readyState === WebSocket.OPEN) ws.send(line + "\n");
  });

  ws.on("message", (data) => {
    // ws delivers binary and text frames as Buffer-ish; coerce to string
    // and strip any trailing newline the sender added so we don't double
    // up before writing to the child.
    const text = data.toString().replace(/\r?\n$/, "");
    if (text.length === 0) return;
    if (DEBUG) console.error(`[bridge:${childPid}] →agent: ${text}`);
    if (agent.stdin.writable) agent.stdin.write(text + "\n");
  });

  // Grace period (ms) between stdin EOF and a forced SIGTERM. The agent
  // should exit on its own once stdin closes; SIGTERM is the backstop for
  // a hung child.
  const SHUTDOWN_GRACE_MS = 5000;

  const shutdown = (cause) => {
    if (cause)
      console.error(`[bridge:${childPid}] ws ${cause}; ending agent stdin`);
    // Close stdin instead of SIGTERMing immediately. Killing the agent on
    // a fast-closing WS (e.g. websocat which closes on stdin EOF without
    // waiting for a response) races with the agent's response write — the
    // SIGTERM can arrive before the agent finishes flushing, and the
    // client sees nothing. Closing stdin signals EOF; the agent finishes
    // any in-flight work, flushes stdout, and exits on its own.
    try {
      agent.stdin.end();
    } catch {
      // Pipe may already be closed; the agent will still be reaped by
      // the grace timer or its natural exit.
    }
    setTimeout(() => {
      if (agent.exitCode === null && agent.signalCode === null) {
        console.error(
          `[bridge:${childPid}] grace period elapsed; sending SIGTERM`,
        );
        try {
          agent.kill("SIGTERM");
        } catch {
          // Already gone.
        }
      }
    }, SHUTDOWN_GRACE_MS).unref();
  };
  ws.on("close", (code, reason) =>
    shutdown(`closed code=${code} reason=${reason?.toString() || "(none)"}`),
  );
  ws.on("error", (err) => shutdown(`errored: ${err?.message ?? err}`));

  agent.on("exit", (code, signal) => {
    console.error(
      `[bridge:${childPid}] agent exited code=${code === null ? "null" : code} signal=${signal === null ? "null" : signal}`,
    );
    if (ws.readyState === WebSocket.OPEN) ws.close();
  });

  agent.on("error", (err) => {
    console.error(`[bridge:${childPid}] failed to spawn agent: ${err.message}`);
    if (ws.readyState === WebSocket.OPEN) ws.close();
  });
});

// Hard cap on graceful shutdown. wss.close() alone only stops accepting
// new connections; existing clients have to disconnect on their own
// before the callback fires. We actively close each open client below,
// which makes our per-connection `ws.on("close")` handler fire and end
// the child agent's stdin — but if a child is wedged, this timer
// guarantees the bridge process still exits instead of hanging forever.
const SHUTDOWN_TIMEOUT_MS = 7000;

let shuttingDown = false;
const stopAll = () => {
  if (shuttingDown) return;
  shuttingDown = true;
  console.error(
    `[bridge] shutting down; closing ${wss.clients.size} open connection(s)`,
  );

  // wss.close() refuses new connections but waits for existing ones to
  // disconnect before its callback. Iterate clients explicitly so each
  // socket actually closes — that's what triggers our per-connection
  // shutdown(), which ends the child's stdin and lets it exit cleanly.
  for (const client of wss.clients) {
    try {
      client.close(1001, "bridge shutting down");
    } catch {
      // Best-effort; fall through to the timeout backstop below.
    }
  }

  wss.close(() => process.exit(0));

  // Backstop: if any child agent hangs past the grace window, force exit
  // so a stuck conversation can't keep the bridge process alive
  // indefinitely. .unref() would let the loop exit early if everything
  // closed cleanly first; here we WANT to keep the timer holding the
  // process up just long enough for graceful shutdown to complete.
  setTimeout(() => {
    console.error(
      `[bridge] shutdown grace exceeded (${SHUTDOWN_TIMEOUT_MS}ms); forcing exit`,
    );
    process.exit(1);
  }, SHUTDOWN_TIMEOUT_MS);
};
process.on("SIGTERM", stopAll);
process.on("SIGINT", stopAll);
