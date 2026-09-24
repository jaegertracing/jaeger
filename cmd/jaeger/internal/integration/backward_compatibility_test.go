// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestRunBackwardCompatibilityTests_SkipsWithoutEnv(t *testing.T) {
	t.Setenv(oldBinaryEnvVar, "")
	t.Setenv(oldConfigDirEnvVar, "")

	// Verify that runBackwardCompatibilityTests skips without panicking when env vars are missing
	runBackwardCompatibilityTests(t, "memory", E2EStorageIntegration{}, compatScenario{
		Name:         "feature gates disabled on both old writer and new reader",
		Capabilities: capabilities.Memory(),
	})
}
