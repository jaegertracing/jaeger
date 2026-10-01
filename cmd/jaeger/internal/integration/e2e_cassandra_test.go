// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestCassandraStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageCassandra)
	s := &E2EStorageIntegration{
		ConfigFile: "../../config-cassandra.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.Cassandra(),
		},
	}
	s.e2eInitialize(t, "cassandra")
	s.RunSpanStoreTests(t)
	// Cassandra's flat tag index cannot run the shared filter battery (testFindTracesWithFilter):
	// every case there is scoped by a service-name disjunction, an operator outside the
	// conjunctive eq-only subset FilterCapabilities declares (RFC 0005 M5). RunFilterRewriteTest
	// is the interim conformance check the RFC calls out for exactly this: a structured filter
	// and the legacy fields it is equivalent to must answer the same way.
	s.RunFilterRewriteTest(t)
}

func TestCassandraStorage_BackwardCompatibility(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageCassandra)
	runBackwardCompatibilityTests(t, "cassandra", E2EStorageIntegration{
		ConfigFile: "../../config-cassandra.yaml",
	}, compatScenario{
		Name:         "feature gates disabled on both old writer and new reader",
		Capabilities: capabilities.Cassandra(),
	})
}
