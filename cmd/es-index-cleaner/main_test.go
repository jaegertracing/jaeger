// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-index-cleaner/app"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

func TestNewESClientForwardsAuth(t *testing.T) {
	tokenFile := func(t *testing.T, content string) string {
		path := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	tests := []struct {
		name      string
		configure func(t *testing.T, cfg *app.Config)
		wantAuth  string
	}{
		{
			name: "basic auth both set sends Authorization",
			configure: func(_ *testing.T, cfg *app.Config) {
				cfg.Username = "user"
				cfg.Password = "pass"
			},
			wantAuth: "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
		},
		{
			name: "basic auth password only omits Authorization",
			configure: func(_ *testing.T, cfg *app.Config) {
				cfg.Password = "pass"
			},
			wantAuth: "",
		},
		{
			name: "bearer token from file",
			configure: func(t *testing.T, cfg *app.Config) {
				cfg.TokenFilePath = tokenFile(t, "my-bearer-token")
			},
			wantAuth: "Bearer my-bearer-token",
		},
		{
			name: "api key from file",
			configure: func(t *testing.T, cfg *app.Config) {
				cfg.APIKeyFilePath = tokenFile(t, "my-api-key")
			},
			wantAuth: "APIKey my-api-key",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The request below is synchronous: GetJaegerIndices returns only
			// after the handler runs and its response is fully read, so this
			// write happens-before the assertion (no data race; verified -race).
			var gotAuth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
			}))
			defer server.Close()

			cfg := &app.Config{}
			test.configure(t, cfg)
			client, err := newESClient(context.Background(), server.URL, cfg, zap.NewNop())
			require.NoError(t, err)

			idx := esclient.IndicesClient{Client: client}
			_, err = idx.GetJaegerIndices(context.Background(), "")
			require.NoError(t, err)
			assert.Equal(t, test.wantAuth, gotAuth)
		})
	}
}

func TestNewESClientRejectsMultipleAuthMethods(t *testing.T) {
	tests := []struct {
		name string
		cfg  app.Config
	}{
		{
			name: "basic and bearer",
			cfg:  app.Config{Username: "u", Password: "p", TokenFilePath: "/token"},
		},
		{
			name: "bearer and api key",
			cfg:  app.Config{TokenFilePath: "/token", APIKeyFilePath: "/apikey"},
		},
		{
			name: "all three",
			cfg:  app.Config{Username: "u", Password: "p", TokenFilePath: "/token", APIKeyFilePath: "/apikey"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.cfg
			_, err := newESClient(context.Background(), "http://localhost:9200", &cfg, zap.NewNop())
			require.ErrorContains(t, err, "only one of")
		})
	}
}
