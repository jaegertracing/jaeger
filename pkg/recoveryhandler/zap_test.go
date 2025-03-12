// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package recoveryhandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestNewRecoveryHandler(t *testing.T) {
	logger, log := testutils.NewLogger()

	handlerFunc := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("Unexpected error!")
	})

	recovery := NewRecoveryHandler(logger, false)(handlerFunc)
	req, err := http.NewRequest(http.MethodGet, "/subdir/asdf", nil)
	require.NoError(t, err)

	res := httptest.NewRecorder()
	recovery.ServeHTTP(res, req)
	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Equal(t, map[string]string{
		"level": "error",
		"msg":   "Unexpected error!",
	}, log.JSONLine(0))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
