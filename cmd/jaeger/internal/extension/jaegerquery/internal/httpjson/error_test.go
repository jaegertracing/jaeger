// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package httpjson

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Length", "1")

	err := WriteError(w, http.StatusBadRequest, map[string]string{"error": "test error"})

	require.NoError(t, err)
	assert.Empty(t, w.Header().Get("Content-Length"))
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"test error"}`, w.Body.String())
}

func TestWriteErrorMarshalFailureDoesNotCommitResponse(t *testing.T) {
	w := &failingResponseWriter{header: http.Header{"Content-Length": []string{"1"}}}

	err := WriteError(w, http.StatusBadRequest, make(chan int))

	require.ErrorContains(t, err, "failed to marshal JSON error response")
	assert.Zero(t, w.statusCode)
	assert.Equal(t, "1", w.Header().Get("Content-Length"))
	assert.Empty(t, w.Header().Get("Content-Type"))
}

func TestWriteErrorWriteFailure(t *testing.T) {
	writeErr := errors.New("write failed")
	w := &failingResponseWriter{header: make(http.Header), writeErr: writeErr}

	err := WriteError(w, http.StatusBadRequest, map[string]string{"error": "test error"})

	require.ErrorIs(t, err, writeErr)
	assert.Equal(t, http.StatusBadRequest, w.statusCode)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}

type failingResponseWriter struct {
	header     http.Header
	statusCode int
	writeErr   error
}

func (w *failingResponseWriter) Header() http.Header {
	return w.header
}

func (w *failingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *failingResponseWriter) Write([]byte) (int, error) {
	return 0, w.writeErr
}
