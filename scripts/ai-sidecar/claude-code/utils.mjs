// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * normalizeWsPayload turns the `ws` library's data shape into a string.
 */
export function normalizeWsPayload(data) {
	if (typeof data === "string") return data;
	if (Buffer.isBuffer(data)) return data.toString("utf8");
	if (data instanceof ArrayBuffer) return Buffer.from(data).toString("utf8");
	if (ArrayBuffer.isView(data)) {
		return Buffer.from(data.buffer, data.byteOffset, data.byteLength).toString(
			"utf8",
		);
	}
	if (Array.isArray(data)) {
		return Buffer.concat(
			data.map((b) => (Buffer.isBuffer(b) ? b : Buffer.from(b))),
		).toString("utf8");
	}
	return String(data);
}

/**
 * injectMcpServers rewrites a JSON-RPC line on its way to the agent.
 */
export function injectMcpServers(line, mcpServers) {
	if (!mcpServers || Object.keys(mcpServers).length === 0) return line;
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

	msg.params._meta = msg.params._meta ?? {};
	msg.params._meta.claudeCode = msg.params._meta.claudeCode ?? {};
	msg.params._meta.claudeCode.options =
		msg.params._meta.claudeCode.options ?? {};
	const options = msg.params._meta.claudeCode.options;
	options.mcpServers = { ...(options.mcpServers ?? {}), ...mcpServers };

	return JSON.stringify(msg);
}
