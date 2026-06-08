// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { Logger } from "./logger.mjs";

/**
 * BridgeConfig encapsulates all configuration for the jaeger-ws-bridge.
 * It handles loading from environment variables, CLI arguments, and
 * performs validation.
 */
export class BridgeConfig {
  static LOOPBACK_HOSTS = new Set(["127.0.0.1", "::1", "localhost"]);

  constructor() {
    this.host = process.env.HOST ?? "127.0.0.1";
    this.port = Number(process.env.PORT ?? 16688);
    this.allowRemote = process.env.ALLOW_REMOTE === "1";
    this.debug = process.env.DEBUG_BRIDGE === "1";
    this.agentEntry = this._resolveAgentEntry();
    this.mcpServers = this._parseMcpServerFlags(process.argv.slice(2));

    // Backpressure thresholds for the agent → ws path.
    this.wsBufferHighWater = 4 * 1024 * 1024;
    this.wsBufferLowWater = 1 * 1024 * 1024;

    // Shutdown grace periods (ms)
    this.shutdownGraceMs = 5000;
    this.shutdownTimeoutMs = 7000;

    this.logger = new Logger({ debug: this.debug });
    this._validate();
  }

  /**
   * Resolve the agent's CLI entry from its package.json.
   */
  _resolveAgentEntry() {
    const pkgJsonPath = fileURLToPath(
      import.meta.resolve("@agentclientprotocol/claude-agent-acp/package.json"),
    );
    const pkg = JSON.parse(readFileSync(pkgJsonPath, "utf8"));
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

  /**
   * Parse repeatable `--mcp-server name=url` flags.
   */
  _parseMcpServerFlags(argv) {
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

  _validate() {
    if (!BridgeConfig.LOOPBACK_HOSTS.has(this.host)) {
      if (!this.allowRemote) {
        this.logger.error(
          `refusing to bind to non-loopback host ${this.host} without ALLOW_REMOTE=1`,
        );
        this.logger.error(
          "the bridge has no auth — exposing it remotely lets any reachable client",
        );
        this.logger.error(
          "drive claude-agent-acp with your local credentials. Set ALLOW_REMOTE=1",
        );
        this.logger.error("to acknowledge the risk and proceed.");
        process.exit(2);
      }
      this.logger.warn(
        `binding to ${this.host} with ALLOW_REMOTE=1; ws://${this.host}:${this.port} is unauthenticated`,
      );
    }
  }
}
