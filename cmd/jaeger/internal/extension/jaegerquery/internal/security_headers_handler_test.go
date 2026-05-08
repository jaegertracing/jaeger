// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeadersHandler(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	handlerToTest := securityHeadersHandler(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	rr := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rr, req)

	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, rr.Header().Get("X-Frame-Options"))
	assert.Empty(t, rr.Header().Get("X-XSS-Protection"))
	assert.Contains(t, rr.Header().Get("Content-Security-Policy"), "default-src 'none'")
	assert.Contains(t, rr.Header().Get("Content-Security-Policy"), "script-src 'self' 'unsafe-inline' 'unsafe-eval'")
	assert.Equal(t, "OK", rr.Body.String())
}

func TestSecurityHeadersHandler_DoesNotOverride(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	handlerToTest := securityHeadersHandler(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	rr := httptest.NewRecorder()

	// Pre-set headers to simulate a proxy or custom config
	rr.Header().Set("X-Content-Type-Options", "some-other-value")
	rr.Header().Set("Content-Security-Policy", "default-src 'self'")

	handlerToTest.ServeHTTP(rr, req)

	assert.Equal(t, "some-other-value", rr.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "default-src 'self'", rr.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "OK", rr.Body.String())
}
