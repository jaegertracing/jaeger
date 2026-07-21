// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/base64"
	"errors"
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

var errActionTest = errors.New("action error")

type dummyAction struct {
	TestFn func() error
}

func (a *dummyAction) Do() error {
	return a.TestFn()
}

func TestExecuteAction(t *testing.T) {
	tests := []struct {
		name                  string
		flags                 []string
		expectedExecuteAction bool
		expectedSkip          bool
		expectedError         error
		actionFunction        func() error
		configError           bool
	}{
		{
			name: "execute errored action",
			flags: []string{
				"--es.tls.skip-host-verify=true",
			},
			expectedExecuteAction: true,
			expectedSkip:          true,
			expectedError:         errActionTest,
		},
		{
			name: "execute success action",
			flags: []string{
				"--es.tls.skip-host-verify=true",
			},
			expectedExecuteAction: true,
			expectedSkip:          true,
			expectedError:         nil,
		},
		{
			name: "don't action because error in tls options",
			flags: []string{
				"--es.tls.cert=/invalid/path/for/cert",
			},
			expectedExecuteAction: false,
			configError:           true,
		},
	}
	logger := zap.NewNop()
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
		res.Write([]byte(`{"version":{"number":"7.0.0"}}`))
	}))
	defer testServer.Close()
	args := []string{
		testServer.URL,
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, command := config.Viperize(AddFlags)
			cmdLine := append([]string{"--es.tls.enabled=true"}, test.flags...)
			require.NoError(t, command.ParseFlags(cmdLine))
			executedAction := false
			err := ExecuteAction(ActionExecuteOptions{
				Args:   args,
				Viper:  v,
				Logger: logger,
			}, func(_ *esclient.Client, _ Config) Action {
				return &dummyAction{
					TestFn: func() error {
						executedAction = true
						return test.expectedError
					},
				}
			})
			assert.Equal(t, test.expectedExecuteAction, executedAction)
			if test.configError {
				require.Error(t, err)
			} else {
				assert.Equal(t, test.expectedError, err)
			}
		})
	}
}

func TestExecuteAction_ConfigError(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	cmdLine := []string{
		"--es.tls.ca=/nonexistent/ca.crt",
	}
	require.NoError(t, command.ParseFlags(cmdLine))
	logger := zap.NewNop()
	args := []string{
		"https://localhost:9300",
	}

	err := ExecuteAction(ActionExecuteOptions{
		Args:   args,
		Viper:  v,
		Logger: logger,
	}, func(_ *esclient.Client, _ Config) Action {
		return &dummyAction{
			TestFn: func() error {
				return nil
			},
		}
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to initialize config")
}

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
			client, err := newESClient(context.Background(), server.URL, cfg, zap.NewNop())
			require.NoError(t, err)

			idx := esclient.IndicesClient{Client: client}
			_, err = idx.GetJaegerIndices(context.Background(), "")
			require.NoError(t, err)
			assert.Equal(t, test.wantAuth, gotAuth)
		})
	}
}

func TestNewESClient_VersionDetectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`)) // response has no version field
	}))
	defer server.Close()

	_, err := newESClient(context.Background(), server.URL, &Config{}, zap.NewNop())
	require.ErrorContains(t, err, "failed to resolve backend version")
}

func TestExecuteAction_ClientError(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	require.NoError(t, command.ParseFlags(nil))
	err := ExecuteAction(ActionExecuteOptions{
		Args:   []string{"not-a-valid-url"}, // no scheme -> esclient.NewClient rejects it
		Viper:  v,
		Logger: zap.NewNop(),
	}, func(*esclient.Client, Config) Action {
		t.Fatal("action must not be created when the client fails")
		return nil
	})
	require.ErrorContains(t, err, "failed to create Elasticsearch client")
}
