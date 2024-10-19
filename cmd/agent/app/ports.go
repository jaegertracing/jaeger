// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

const (
	// AgentJaegerThriftCompactUDP is the default port for receiving Jaeger Thrift over UDP in compact encoding
	AgentJaegerThriftCompactUDP = 6831
	// AgentJaegerThriftBinaryUDP is the default port for receiving Jaeger Thrift over UDP in binary encoding
	AgentJaegerThriftBinaryUDP = 6832
	// AgentZipkinThriftCompactUDP is the default port for receiving Zipkin Thrift over UDP in binary encoding
	AgentZipkinThriftCompactUDP = 5775
	// AgentConfigServerHTTP is the default port for the agent's HTTP config server (e.g. /sampling endpoint)
	AgentConfigServerHTTP = 5778
	// AgentAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	AgentAdminHTTP = 14271
)
