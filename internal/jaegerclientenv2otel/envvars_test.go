// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerclientenv2otel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMapJaegerToOtelEnvVars(t *testing.T) {
	t.Setenv("JAEGER_TAGS", "tags")
	t.Setenv("JAEGER_USER", "user")

	logger, buffer := testutils.NewLogger()
	MapJaegerToOtelEnvVars(logger)

	assert.Equal(t, "user", os.Getenv("OTEL_EXPORTER_JAEGER_USER"))
	assert.Contains(t, buffer.String(), "Replacing deprecated Jaeger SDK env var JAEGER_USER with OpenTelemetry env var OTEL_EXPORTER_JAEGER_USER")
	assert.Contains(t, buffer.String(), "Ignoring deprecated Jaeger SDK env var JAEGER_TAGS")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
