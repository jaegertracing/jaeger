// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhousetest

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postQuery(t *testing.T, url, query string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "text/plain", strings.NewReader(query))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestNewServer_PingQuery(t *testing.T) {
	srv := NewServer(FailureConfig{})
	defer srv.Close()
	status, _ := postQuery(t, srv.URL, PingQuery)
	assert.Equal(t, http.StatusOK, status)
}

func TestNewServer_HandshakeQuery(t *testing.T) {
	srv := NewServer(FailureConfig{})
	defer srv.Close()
	status, _ := postQuery(t, srv.URL, HandshakeQuery)
	assert.Equal(t, http.StatusOK, status)
}

func TestNewServer_DefaultQuery(t *testing.T) {
	srv := NewServer(FailureConfig{})
	defer srv.Close()
	status, _ := postQuery(t, srv.URL, "SELECT * FROM some_table")
	assert.Equal(t, http.StatusOK, status)
}

func TestNewServer_FailureConfig(t *testing.T) {
	wantErr := errors.New("simulated failure")
	srv := NewServer(FailureConfig{PingQuery: wantErr})
	defer srv.Close()
	status, body := postQuery(t, srv.URL, PingQuery)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Contains(t, body, wantErr.Error())
}
