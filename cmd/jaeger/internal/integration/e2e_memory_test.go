// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestMemoryStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageMemoryV2)

	featureGates := append([]string{},
		querysvc.StructuredFiltersGate.ID(),
	)
	featureGates = append(featureGates, paginationGates...)

	s := &E2EStorageIntegration{
		ConfigFile:   "../../config.yaml",
		FeatureGates: featureGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.Capabilities{}.WithoutSpanAttributeOrdering(),
		},
	}
	s.e2eInitialize(t, "memory")
	s.RunAll(t)
	// Verify that querying via the legacy predicate fields and via the equivalent
	// structured filter return identical results against a live backend.
	s.RunFilterRewriteTest(t)
}
