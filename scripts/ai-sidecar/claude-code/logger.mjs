// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * A simple structured logger for the Jaeger WS Bridge.
 * Routes INFO/DEBUG to stdout and WARN/ERROR to stderr.
 */
export class Logger {
	constructor(options = { debug: false }) {
		this.isDebugEnabled = options.debug;
	}

	_log(level, ...args) {
		const timestamp = new Date().toISOString();
		const prefix = `[${timestamp}] [${level}]`;

		if (level === "ERROR" || level === "WARN") {
			console.error(prefix, ...args);
		} else {
			console.log(prefix, ...args);
		}
	}

	info(...args) {
		this._log("INFO", ...args);
	}

	warn(...args) {
		this._log("WARN", ...args);
	}

	error(...args) {
		this._log("ERROR", ...args);
	}

	debug(...args) {
		if (this.isDebugEnabled) {
			this._log("DEBUG", ...args);
		}
	}

	/**
	 * Creates a namespaced logger (e.g., for a specific connection).
	 */
	child(namespace) {
		return {
			info: (...args) => this.info(`[${namespace}]`, ...args),
			warn: (...args) => this.warn(`[${namespace}]`, ...args),
			error: (...args) => this.error(`[${namespace}]`, ...args),
			debug: (...args) => this.debug(`[${namespace}]`, ...args),
		};
	}
}
