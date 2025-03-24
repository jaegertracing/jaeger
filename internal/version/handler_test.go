// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRegisterHandler(t *testing.T) {
	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2024-01-04"
	expectedJSON := `{"gitCommit":"foobar","gitVersion":"v1.2.3","buildDate":"2024-01-04"}`

	mockLogger := zap.NewNop()
	mux := http.NewServeMux()
	RegisterHandler(mux, mockLogger)
	server := httptest.NewServer(mux)

	defer server.Close()

	resp, err := http.Get(server.URL + "/version")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	assert.JSONEq(t, expectedJSON, string(body))
}
