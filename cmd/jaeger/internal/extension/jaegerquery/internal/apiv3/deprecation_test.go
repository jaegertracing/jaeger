// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/http/httptest"
	"testing"

	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/stretchr/testify/assert"
)

func TestApplyDeprecationHeaders_presentWhenDeprecatedUsed(t *testing.T) {
	w := httptest.NewRecorder()
	applyDeprecationHeaders(w, []string{paramStartTimeDeprecated})

	assert.Equal(t, "true", w.Header().Get("Deprecation"))
	assert.Equal(t, deprecatedParamsSunset, w.Header().Get("Sunset"))
	assert.Contains(t, w.Header().Get("Link"), "rel=\"deprecation\"")
	assert.Equal(t, paramStartTimeDeprecated, w.Header().Get("Deprecated-Params"))
}

func TestApplyDeprecationHeaders_absentWhenNoDeprecated(t *testing.T) {
	w := httptest.NewRecorder()
	applyDeprecationHeaders(w, nil)

	assert.Empty(t, w.Header().Get("Deprecation"))
	assert.Empty(t, w.Header().Get("Sunset"))
	assert.Empty(t, w.Header().Get("Deprecated-Params"))
}

func TestLogDeprecatedParams_emitsSingleWarnPerRequest(t *testing.T) {
	logger, log := testutils.NewLogger()
	logDeprecatedParams(logger, "127.0.0.1:1234", []string{paramStartTimeDeprecated, paramSpanKindDeprecated})

	assert.Contains(t, log.String(), "deprecated API v3 query parameter used")
	assert.Contains(t, log.String(), paramStartTimeDeprecated)
	assert.Contains(t, log.String(), paramSpanKindDeprecated)
}

func TestLogDeprecatedParams_noLogWhenCanonicalOnly(t *testing.T) {
	logger, log := testutils.NewLogger()
	logDeprecatedParams(logger, "127.0.0.1:1234", nil)
	assert.Empty(t, log.String())
}
