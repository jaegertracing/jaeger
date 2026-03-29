// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

// The structs below are duplicated from cmd/jaeger/internal/extension/jaegerquery/internal/ddg
// because the original package is marked as internal to jaegerquery and thus cannot be imported
// by the integration package due to Go's internal package visibility rules.
type TDdgPayloadEntry struct {
	Service   string `json:"service"`
	Operation string `json:"operation"`
}

type Attribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TDdgPayloadPath struct {
	Path       []TDdgPayloadEntry `json:"path"`
	Attributes []Attribute        `json:"attributes"`
}

type TDdgPayload struct {
	Dependencies []TDdgPayloadPath `json:"dependencies"`
}

func TestDeepDependenciesEndpoint(t *testing.T) {
	integration.SkipUnlessEnv(t, "memory_v2")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config.yaml",
	}
	s.e2eInitialize(t, "memory")

	baseURL := fmt.Sprintf("http://localhost:%d/api", ports.QueryHTTP)

	t.Run("no_service_param_uses_default", func(t *testing.T) {
		resp := queryDeepDependencies(t, baseURL, "")
		require.NotEmpty(t, resp.Dependencies, "expected non-empty dependencies with default focal service")

		// When no service is provided, the mock implementation defaults to "customer".
		defaultFocalService := "customer"
		found := false
		for _, path := range resp.Dependencies {
			for _, entry := range path.Path {
				if entry.Service == defaultFocalService {
					found = true
				}
			}
		}
		assert.True(t, found, "default focal service %q should appear in at least one dependency path when no service param is provided", defaultFocalService)
	})

	t.Run("with_service_param", func(t *testing.T) {
		resp := queryDeepDependencies(t, baseURL, "order-service")
		require.NotEmpty(t, resp.Dependencies, "expected non-empty dependencies for focal service")

		// Verify the focal service appears somewhere in the returned paths.
		found := false
		for _, path := range resp.Dependencies {
			for _, entry := range path.Path {
				if entry.Service == "order-service" {
					found = true
				}
			}
		}
		assert.True(t, found, "focal service 'order-service' should appear in at least one dependency path")
	})

	t.Run("each_path_has_attributes", func(t *testing.T) {
		resp := queryDeepDependencies(t, baseURL, "")
		for i, path := range resp.Dependencies {
			assert.NotEmptyf(t, path.Path, "dependency path %d should have at least one entry", i)
			assert.NotEmptyf(t, path.Attributes, "dependency path %d should have at least one attribute", i)
		}
	})
}

// queryDeepDependencies calls GET /api/deep-dependencies with an optional service
// query parameter and returns the parsed TDdgPayload.
func queryDeepDependencies(t *testing.T, baseURL string, service string) TDdgPayload {
	t.Helper()

	urlString := baseURL + "/deep-dependencies"
	if service != "" {
		urlString += "?service=" + url.QueryEscape(service)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlString, http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode,
		"expected HTTP 200 from %s", urlString)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var payload TDdgPayload
	require.NoError(t, json.Unmarshal(body, &payload),
		"response body should be valid JSON: %s", string(body))

	return payload
}
