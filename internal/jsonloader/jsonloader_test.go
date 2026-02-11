// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jsonloader

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestLoadJSON_Success(t *testing.T) {
	loadFn := func() ([]byte, error) {
		return []byte(`{"name":"test","value":42}`), nil
	}
	result, err := LoadJSON[testConfig](loadFn)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Value)
}

func TestLoadJSON_LoaderError(t *testing.T) {
	expectedErr := errors.New("failed to load")
	loadFn := func() ([]byte, error) {
		return nil, expectedErr
	}
	result, err := LoadJSON[testConfig](loadFn)
	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, result)
}

func TestLoadJSON_InvalidJSON(t *testing.T) {
	loadFn := func() ([]byte, error) {
		return []byte(`not valid json`), nil
	}
	result, err := LoadJSON[testConfig](loadFn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
	assert.Nil(t, result)
}

func TestLoadJSON_NullJSON(t *testing.T) {
	loadFn := func() ([]byte, error) {
		return []byte(`null`), nil
	}
	result, err := LoadJSON[testConfig](loadFn)
	require.NoError(t, err)
	assert.Nil(t, result)
}
