// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ports

import (
	"strconv"
	"strings"
)

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

	// CollectorGRPC is the default port for gRPC server for sending spans
	CollectorGRPC = 14250
	// CollectorHTTP is the default port for HTTP server for sending spans (e.g. /api/traces endpoint)
	CollectorHTTP = 14268
	// CollectorAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	CollectorAdminHTTP = 14269

	// QueryGRPC is the default port of GRPC requests for Query trace retrieval
	QueryGRPC = 16685
	// QueryHTTP is the default port for UI and Query API (e.g. /api/* endpoints)
	QueryHTTP = 16686
	// QueryAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	QueryAdminHTTP = 16687

	// IngesterAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	IngesterAdminHTTP = 14270
)

// PortToHostPort converts the port into a host:port address string
func PortToHostPort(port int) string {
	return ":" + strconv.Itoa(port)
}

// GetAddressFromCLIOptions gets listening address based on port (deprecated flags) or host:port (new flags)
func GetAddressFromCLIOptions(port int, hostPort string) string {
	if port != 0 {
		return PortToHostPort(port)
	}

	if hostPort == "" {
		return ""
	}

	if strings.Contains(hostPort, ":") {
		return hostPort
	}

	return ":" + hostPort
}
