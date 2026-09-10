// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func writeFile(t *testing.T, content string) string {
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestInitBasicAuth_StaticPassword(t *testing.T) {
	m, err := initBasicAuth(&config.BasicAuthentication{Username: "user", Password: "pass"}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "Basic", m.Scheme)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("user:pass")), m.TokenFn())
}

func TestInitBasicAuth_PasswordFromFile(t *testing.T) {
	m, err := initBasicAuth(&config.BasicAuthentication{
		Username:         "user",
		PasswordFilePath: writeFile(t, "filepass"),
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("user:filepass")), m.TokenFn())
}

func TestInitBasicAuth_Errors(t *testing.T) {
	t.Run("password and file both set", func(t *testing.T) {
		_, err := initBasicAuth(&config.BasicAuthentication{Username: "u", Password: "p", PasswordFilePath: "/x"}, zap.NewNop())
		require.ErrorContains(t, err, "both Password and PasswordFilePath")
	})
	t.Run("password file missing", func(t *testing.T) {
		_, err := initBasicAuth(&config.BasicAuthentication{Username: "u", PasswordFilePath: "/nonexistent"}, zap.NewNop())
		require.Error(t, err)
	})
	t.Run("empty username yields no method", func(t *testing.T) {
		m, err := initBasicAuth(&config.BasicAuthentication{Password: "p"}, zap.NewNop())
		require.NoError(t, err)
		assert.Nil(t, m)
	})
}

func TestInitTokenAuth_FromFile(t *testing.T) {
	path := writeFile(t, "the-token")

	bearer, err := initBearerAuth(&config.TokenAuthentication{FilePath: path}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "Bearer", bearer.Scheme)
	assert.Equal(t, "the-token", bearer.TokenFn())

	apiKey, err := initAPIKeyAuth(&config.TokenAuthentication{FilePath: path}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, "APIKey", apiKey.Scheme)
	assert.Equal(t, "the-token", apiKey.TokenFn())
}

func TestInitTokenAuth_FromContext(t *testing.T) {
	m, err := initAPIKeyAuth(&config.TokenAuthentication{AllowFromContext: true}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "APIKey", m.Scheme)
	assert.NotNil(t, m.FromCtx)
}

func TestInitTokenAuth_FileMissing(t *testing.T) {
	_, err := initBearerAuth(&config.TokenAuthentication{FilePath: "/nonexistent"}, zap.NewNop())
	require.Error(t, err)
}
