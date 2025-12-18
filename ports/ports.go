// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ports

import (
	"strconv"
)

const (
	// CollectorV2GRPC is the HTTP port for remote sampling extension
	CollectorV2SamplingHTTP = 5778
	// CollectorV2GRPC is the gRPC port for remote sampling extension
	CollectorV2SamplingGRPC = 5779
	// CollectorV2HealthChecks is the port for health checks extension
	CollectorV2HealthChecks = 13133

	// QueryGRPC is the default port of GRPC requests for Query trace retrieval
	QueryGRPC = 16685
	// QueryHTTP is the default port for UI and Query API (e.g. /api/* endpoints)
	QueryHTTP = 16686

	// RemoteStorageGRPC is the default port of GRPC requests for Remote Storage
	RemoteStorageGRPC = 17271
	// RemoteStorageHTTP is the default admin HTTP port (health check, metrics, etc.)
	RemoteStorageAdminHTTP = 17270
)

// PortToHostPort converts the port into a host:port address string
func PortToHostPort(port int) string {
	return ":" + strconv.Itoa(port)
}
