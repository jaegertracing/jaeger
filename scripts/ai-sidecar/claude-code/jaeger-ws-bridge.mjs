// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
//
// jaeger-ws-bridge: a WS→stdio bridge that lets Jaeger's AI gateway use
// claude-agent-acp as its ACP sidecar.

import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import { WebSocket, WebSocketServer } from "ws";
import { BridgeConfig } from "./config.mjs";
import { injectMcpServers, normalizeWsPayload } from "./utils.mjs";

let config;
try {
	config = new BridgeConfig();
} catch (err) {
	console.error(`[ERROR] ${err.message}`);
	process.exit(2);
}
const logger = config.logger;
const wss = new WebSocketServer({ host: config.host, port: config.port });

logger.info(`listening on ws://${config.host}:${config.port}`);
logger.info(`agent entry: ${config.agentEntry}`);
if (Object.keys(config.mcpServers).length > 0) {
	logger.info(
		`injecting MCP servers on session/new: ${Object.keys(config.mcpServers).join(", ")}`,
	);
}

const activeAgents = new Set();

wss.on("connection", (ws) => {
	const agent = spawn(process.execPath, [config.agentEntry], {
		stdio: ["pipe", "pipe", "pipe"],
		env: process.env,
	});
	activeAgents.add(agent);
	const childPid = agent.pid;
	const connLogger = logger.child(`bridge:${childPid}`);
	const agentLogger = logger.child(`agent:${childPid}`);

	connLogger.info("connection opened");

	const stderrLines = createInterface({ input: agent.stderr });
	stderrLines.on("line", (line) => {
		agentLogger.error(line);
	});

	let backpressureTimer = null;
	const resumeIfDrained = () => {
		if (backpressureTimer) {
			clearInterval(backpressureTimer);
			backpressureTimer = null;
		}
		if (agent.stdout.isPaused()) agent.stdout.resume();
	};

	const applyBackpressure = () => {
		if (backpressureTimer) return;
		if (!agent.stdout.isPaused()) agent.stdout.pause();
		backpressureTimer = setInterval(() => {
			if (
				ws.readyState !== WebSocket.OPEN ||
				ws.bufferedAmount <= config.wsBufferLowWater
			) {
				resumeIfDrained();
			}
		}, 50);
		backpressureTimer.unref();
	};

	const lines = createInterface({ input: agent.stdout });
	lines.on("line", (line) => {
		if (line.length === 0) return;
		connLogger.debug(`→ws: ${line}`);
		if (ws.readyState === WebSocket.OPEN) {
			ws.send(`${line}\n`);
			if (ws.bufferedAmount > config.wsBufferHighWater) applyBackpressure();
		}
	});

	let wsInboundBuffer = "";
	ws.on("message", (data) => {
		wsInboundBuffer += normalizeWsPayload(data);

		if (wsInboundBuffer.length > config.maxInboundLineLength) {
			connLogger.error(
				`inbound message exceeded maximum line length (${config.maxInboundLineLength}); closing connection to prevent memory exhaustion`,
			);
			ws.close(1009, "Message Too Big");
			wsInboundBuffer = "";
			return;
		}

		const parts = wsInboundBuffer.split(/\r?\n/);
		// Last element is either "" (payload ended with \n) or a partial line;
		// retain it for the next message event.
		wsInboundBuffer = parts.pop();
		for (const line of parts) {
			if (line.length === 0) continue;
			const text = injectMcpServers(line, config.mcpServers);
			connLogger.debug(`→agent: ${text}`);
			if (agent.stdin.writable) agent.stdin.write(`${text}\n`);
		}
	});

	const shutdownAgent = (cause) => {
		if (cause) connLogger.info(`ws ${cause}; ending agent stdin`);
		try {
			agent.stdin.end();
		} catch {
			// Pipe already closed
		}
		setTimeout(() => {
			if (agent.exitCode === null && agent.signalCode === null) {
				connLogger.warn("grace period elapsed; sending SIGTERM");
				try {
					agent.kill("SIGTERM");
				} catch {
					// Already gone
				}
			}
		}, config.shutdownGraceMs).unref();
	};

	ws.on("close", (code, reason) =>
		shutdownAgent(
			`closed code=${code} reason=${reason?.toString() || "(none)"}`,
		),
	);
	ws.on("error", (err) => shutdownAgent(`errored: ${err?.message ?? err}`));

	agent.on("exit", (code, signal) => {
		activeAgents.delete(agent);
		connLogger.info(
			`agent exited code=${code ?? "null"} signal=${signal ?? "null"}`,
		);
		if (ws.readyState === WebSocket.OPEN) ws.close();
	});

	agent.on("error", (err) => {
		activeAgents.delete(agent);
		connLogger.error(`failed to spawn agent: ${err.message}`);
		if (ws.readyState === WebSocket.OPEN) ws.close();
	});
});

let shuttingDown = false;
const stopAll = () => {
	if (shuttingDown) return;
	shuttingDown = true;
	logger.info(
		`shutting down; closing ${wss.clients.size} open connection(s) and killing ${activeAgents.size} agent(s)`,
	);

	for (const client of wss.clients) {
		try {
			client.close(1001, "bridge shutting down");
		} catch {
			// Best-effort
		}
	}

	for (const agent of activeAgents) {
		try {
			agent.kill("SIGTERM");
		} catch {
			// Best-effort
		}
	}

	wss.close(() => process.exit(0));

	setTimeout(() => {
		logger.error(
			`shutdown grace exceeded (${config.shutdownGraceMs}ms); forcing exit`,
		);
		for (const client of wss.clients) {
			try {
				client.terminate();
			} catch {
				// Best-effort
			}
		}
		for (const agent of activeAgents) {
			try {
				agent.kill("SIGKILL");
			} catch {
				// Best-effort
			}
		}
		process.exit(1);
	}, config.shutdownGraceMs).unref();
};

process.on("SIGTERM", stopAll);
process.on("SIGINT", stopAll);
