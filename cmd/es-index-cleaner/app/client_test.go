// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

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

	"github.com/jaegertracing/jaeger/internal/config"
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
		configure func(t *testing.T, cfg *Config)
		wantAuth  string
	}{
		{
			name: "basic auth both set sends Authorization",
			configure: func(_ *testing.T, cfg *Config) {
				cfg.Username = "user"
				cfg.Password = "pass"
			},
			wantAuth: "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
		},
		{
			name: "basic auth password only omits Authorization",
			configure: func(_ *testing.T, cfg *Config) {
				cfg.Password = "pass"
			},
			wantAuth: "",
		},
		{
			name: "bearer token from file",
			configure: func(t *testing.T, cfg *Config) {
				cfg.TokenFilePath = tokenFile(t, "my-bearer-token")
			},
			wantAuth: "Bearer my-bearer-token",
		},
		{
			name: "api key from file",
			configure: func(t *testing.T, cfg *Config) {
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
				if r.URL.Path == "/" { // construction-time version probe
					w.Write([]byte(`{"version":{"number":"8.0.0"},"tagline":"You Know, for Search"}`))
					return
				}
				w.Write([]byte("{}"))
			}))
			defer server.Close()

			cfg := &Config{}
			test.configure(t, cfg)
			client, err := NewESClient(context.Background(), server.URL, cfg, zap.NewNop())
			require.NoError(t, err)

			idx := esclient.IndicesClient{Client: client}
			_, err = idx.GetJaegerIndices(context.Background(), "")
			require.NoError(t, err)
			assert.Equal(t, test.wantAuth, gotAuth)
		})
	}
}

func TestNewESClient_RejectsMultipleAuthMethods(t *testing.T) {
	tests := map[string][]string{
		"basic and bearer":   {"--es.username=u", "--es.password=p", "--es.token-file=/token"},
		"bearer and api key": {"--es.token-file=/token", "--es.api-key-file=/apikey"},
		"all three":          {"--es.username=u", "--es.password=p", "--es.token-file=/token", "--es.api-key-file=/apikey"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("no request should be sent for an invalid config")
			}))
			defer server.Close()

			cfg := &Config{}
			v, command := config.Viperize(cfg.AddFlags)
			require.NoError(t, command.ParseFlags(args))
			require.NoError(t, cfg.InitFromViper(v))

			_, err := NewESClient(context.Background(), server.URL, cfg, zap.NewNop())
			require.ErrorContains(t, err, "at most one authentication method")
		})
	}
}
