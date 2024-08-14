// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerclientenv2otel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMapJaegerToOtelEnvVars(t *testing.T) {
	os.Setenv("JAEGER_TAGS", "tags")
	os.Setenv("JAEGER_USER", "user")
	os.Unsetenv("OTEL_EXPORTER_JAEGER_USER")

	logger, buffer := testutils.NewLogger()
	MapJaegerToOtelEnvVars(logger)

	assert.Equal(t, "user", os.Getenv("OTEL_EXPORTER_JAEGER_USER"))
	assert.Contains(t, buffer.String(), "Replacing deprecated Jaeger SDK env var JAEGER_USER with OpenTelemetry env var OTEL_EXPORTER_JAEGER_USER")
	assert.Contains(t, buffer.String(), "Ignoring deprecated Jaeger SDK env var JAEGER_TAGS")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
