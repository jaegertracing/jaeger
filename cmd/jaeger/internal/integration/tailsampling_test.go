// // Copyright (c) 2024 The Jaeger Authors.
// // SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"os/exec"
	"sort"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TailSamplingIntegration contains the test components to perform an integration test
// for the Tail Sampling Processor.
type TailSamplingIntegration struct {
	E2EStorageIntegration

	// expectedServices contains a list of services that should be sampled in the test case.
	expectedServices []string
}

// TestTailSamplingProcessor_EnforcesPolicies runs an A/B test to perform an integration test
// for the Tail Sampling Processor.
//   - Test A uses a Jaeger config file with a tail sampling processor that has a policy for sampling
//     all traces. In this test, we check that all services that are samples are stored.
//   - Test B uses a Jaeger config file with a tail sampling processor that has a policy to sample
//     traces using on the `service.name` attribute. In this test, we check that only the services
//     listed as part of the policy in the config file are stored.
func TestTailSamplingProcessor_EnforcesPolicies(t *testing.T) {
	env := os.Getenv("SAMPLING")
	if env != "tail" {
		t.Skipf("This test requires environment variable SAMPLING=tail")
	}

	expectedServicesA := []string{"tracegen-00", "tracegen-01", "tracegen-02", "tracegen-03", "tracegen-04"}
	tailSamplingA := &TailSamplingIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: "../../config-tail-sampling-always-sample.yaml",
			StorageIntegration: integration.StorageIntegration{
				CleanUp: purge,
			},
		},
		expectedServices: expectedServicesA,
	}

	expectedServicesB := []string{"tracegen-00", "tracegen-03"}
	tailSamplingB := &TailSamplingIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: "../../config-tail-sampling-service-name-policy.yaml",
			StorageIntegration: integration.StorageIntegration{
				CleanUp: purge,
			},
		},
		expectedServices: expectedServicesB,
	}

	t.Run("testA", tailSamplingA.testTailSamplingProccessor)
	t.Run("testB", tailSamplingB.testTailSamplingProccessor)
}

// testTailSamplingProccessor performs the following steps:
//  1. Initialize the test case by starting the Jaeger V2 collector
//  2. Generate 5 traces using `tracegen` with one service per trace
//  3. Read the stored services from the memory store
//  4. Check that the sampled services match what is expected
func (ts *TailSamplingIntegration) testTailSamplingProccessor(t *testing.T) {
	ts.e2eInitialize(t, "memory")
	ts.generateTraces(t)

	var actual []string
	found := ts.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = ts.SpanReader.GetServices(context.Background())
		require.NoError(t, err)
		sort.Strings(actual)
		return assert.ObjectsAreEqualValues(ts.expectedServices, actual)
	})

	if !assert.True(t, found) {
		t.Log("\t Expected:", ts.expectedServices)
		t.Log("\t Actual  :", actual)
	}
}

// generateTraces generates 5 traces using `tracegen` with one service per trace
func (*TailSamplingIntegration) generateTraces(t *testing.T) {
	os.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://localhost:4318/v1/traces")
	tracegenCmd := exec.Command("go", "run", "../../../../cmd/tracegen", "-traces", "5", "-services", "5")
	stdout, err := tracegenCmd.Output()
	require.NoError(t, err)
	t.Logf("tracegen completed: %s", stdout)
}

func (*TailSamplingIntegration) waitForCondition(t *testing.T, predicate func(t *testing.T) bool) bool {
	const iterations = 100 // Will wait at most 100 seconds.
	for i := 0; i < iterations; i++ {
		if predicate(t) {
			return true
		}
		t.Logf("Waiting for storage backend to update documents, iteration %d out of %d", i+1, iterations)
		time.Sleep(time.Second)
	}
	return predicate(t)
}
