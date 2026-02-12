// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jio

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

func TestJSONLoad_Success(t *testing.T) {
	loader := func() ([]byte, error) {
		return []byte(`{"name":"test","value":42}`), nil
	}

	result, err := JSONLoad[testConfig](loader)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Value)
}

func TestJSONLoad_NullJSON(t *testing.T) {
	loader := func() ([]byte, error) {
		return []byte("null"), nil
	}

	result, err := JSONLoad[testConfig](loader)
	require.NoError(t, err)
	assert.Zero(t, result.Name)
	assert.Zero(t, result.Value)
}

func TestJSONLoad_LoaderError(t *testing.T) {
	loader := func() ([]byte, error) {
		return nil, errors.New("load failed")
	}

	result, err := JSONLoad[testConfig](loader)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, "load failed", err.Error())
}

func TestJSONLoad_InvalidJSON(t *testing.T) {
	loader := func() ([]byte, error) {
		return []byte(`{invalid}`), nil
	}

	result, err := JSONLoad[testConfig](loader)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to unmarshal JSON")
}

func TestJSONLoad_PointerType(t *testing.T) {
	loader := func() ([]byte, error) {
		return []byte(`{"name":"ptr","value":99}`), nil
	}

	result, err := JSONLoad[*testConfig](loader)
	require.NoError(t, err)
	require.NotNil(t, *result)
	assert.Equal(t, "ptr", (*result).Name)
	assert.Equal(t, 99, (*result).Value)
}

func TestJSONLoad_NullPointerType(t *testing.T) {
	loader := func() ([]byte, error) {
		return []byte("null"), nil
	}

	result, err := JSONLoad[*testConfig](loader)
	require.NoError(t, err)
	assert.Nil(t, *result)
}
