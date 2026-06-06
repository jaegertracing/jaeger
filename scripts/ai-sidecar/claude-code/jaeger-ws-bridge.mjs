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
import { readFileSync } from "node:fs";
import { createInterface } from "node:readline";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { WebSocket, WebSocketServer } from "ws";

// Resolve the agent's CLI entry from its package.json so the bridge keeps
// working when claude-agent-acp's dist layout changes between releases.
// `import.meta.resolve` + readFileSync replaces an older createRequire
// shim — both return the same path, but the static-ESM form avoids
// pulling the CommonJS require helper into a pure-ESM bridge.
function resolveAgentEntry() {
  const pkgJsonPath = fileURLToPath(
    import.meta.resolve("@agentclientprotocol/claude-agent-acp/package.json"),
  );
  const pkg = JSON.parse(readFileSync(pkgJsonPath, "utf8"));
  // pkg.bin can be a string ("./dist/index.js"), a map keyed by bin name,
  // or absent. When it's a map we prefer the entry keyed by our expected
  // unscoped name; if the package picks a different key (renamed bin,
  // monorepo, etc.) we fall back to the first declared value so the
  // bridge keeps working without an upstream patch.
  let binEntry;
  if (typeof pkg.bin === "string") {
    binEntry = pkg.bin;
  } else if (pkg.bin && typeof pkg.bin === "object") {
    binEntry = pkg.bin["claude-agent-acp"] ?? Object.values(pkg.bin)[0];
  }
  if (typeof binEntry !== "string" || binEntry.length === 0) {
    throw new Error(
      "claude-agent-acp package.json does not declare a runnable bin entry",
    );
  }
  return resolve(dirname(pkgJsonPath), binEntry);
}

const AGENT_ENTRY = resolveAgentEntry();
const HOST = process.env.HOST ?? "127.0.0.1";
const PORT = Number(process.env.PORT ?? 16688);

// Parse repeatable `--mcp-server name=url` flags into a Claude-shaped MCP
// servers map. claude-agent-acp itself does NOT expose this flag — it
// only honors MCP entries that arrive via NewSessionRequest._meta or its
// own SDK settings files. Putting the flag here lets operators wire MCP
// without editing ~/.claude/settings.json, while still keeping the
// gateway out of the MCP egress path (Yuri's inversion-of-control point
// in jaegertracing/jaeger#8631 — the bridge is part of the operator's
// trust domain, the upstream agent is not).
function parseMcpServerFlags(argv) {
  const entries = {};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] !== "--mcp-server") continue;
    const spec = argv[i + 1];
    if (!spec || !spec.includes("=")) {
      throw new Error(
        `--mcp-server expects name=url, got ${JSON.stringify(spec)}`,
      );
    }
    const eq = spec.indexOf("=");
    const name = spec.slice(0, eq).trim();
    const url = spec.slice(eq + 1).trim();
    if (!name || !url) {
      throw new Error(
        `--mcp-server expects non-empty name and url, got ${JSON.stringify(spec)}`,
      );
    }
    let parsed;
    try {
      parsed = new URL(url);
    } catch {
      throw new Error(`--mcp-server ${name}=${url} is not a valid URL`);
    }
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      throw new Error(`--mcp-server ${name}=${url} must use http(s) scheme`);
    }
    entries[name] = { type: "http", url };
    i++;
  }
  return entries;
}

const MCP_SERVERS = parseMcpServerFlags(process.argv.slice(2));

const wss = new WebSocketServer({ host: HOST, port: PORT });
console.error(`[bridge] listening on ws://${HOST}:${PORT}`);
console.error(`[bridge] agent entry: ${AGENT_ENTRY}`);
if (Object.keys(MCP_SERVERS).length > 0) {
  console.error(
    `[bridge] injecting MCP servers on session/new: ${Object.keys(MCP_SERVERS).join(", ")}`,
  );
}

// Set DEBUG_BRIDGE=1 to log every JSON-RPC message in both directions.
// Off by default to keep production logs uncluttered.
const DEBUG = process.env.DEBUG_BRIDGE === "1";

// injectMcpServers rewrites a JSON-RPC line on its way to the agent. For
// `session/new` requests it merges the bridge's --mcp-server entries into
// `params._meta.claudeCode.options.mcpServers`, which is the field the
// Claude Agent SDK reads via `userProvidedOptions?.mcpServers` (see
// claude-agent-acp src/acp-agent.ts:1925, 1979). Any entries the gateway
// already put in that field win — bridge-supplied ones are defaults so
// future gateway pushes don't get silently overwritten by the operator's
// CLI. Anything that fails to parse passes through untouched so a
// malformed frame can't be turned into a bigger failure by the bridge.
function injectMcpServers(line) {
  if (Object.keys(MCP_SERVERS).length === 0) return line;
  if (!line.includes('"session/new"')) return line;
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return line;
  }
  if (
    msg?.method !== "session/new" ||
    typeof msg.params !== "object" ||
    msg.params === null
  ) {
    return line;
  }
  const meta = (msg.params._meta ??= {});
  const claudeCode = (meta.claudeCode ??= {});
  const options = (claudeCode.options ??= {});
  options.mcpServers = { ...MCP_SERVERS, ...(options.mcpServers ?? {}) };
  return JSON.stringify(msg);
}

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
    // and split on newlines. The gateway typically sends one JSON-RPC
    // message per WS frame, but ACP framing on the agent's stdin is
    // line-delimited, so multiple messages concatenated with `\n` inside
    // a single frame are still legal — handle them by writing each
    // non-empty line individually and re-appending the terminator
    // exactly once. Symmetric with the agent→ws path (createInterface
    // also splits on newlines), so behaviour is consistent in both
    // directions even if some sender starts batching.
    for (const line of data.toString().split(/\r?\n/)) {
      if (line.length === 0) continue;
      const text = injectMcpServers(line);
      if (DEBUG) console.error(`[bridge:${childPid}] →agent: ${text}`);
      if (agent.stdin.writable) agent.stdin.write(text + "\n");
    }
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
