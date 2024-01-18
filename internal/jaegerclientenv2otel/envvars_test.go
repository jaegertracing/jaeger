// Copyright (c) 2023 The Jaeger Authors.
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
