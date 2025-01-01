// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esrollover

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	argumentsError = "accepts 1 arg(s), received 0"
)

func TestCommand(t *testing.T) {
	v := viper.New()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	cmd := Command(v, logger)
	assert.NotNil(t, cmd)
	assert.Equal(t, "es-rollover", cmd.Use)
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestAllCommands(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "init",
			expected: "Get \"http://localhost:9200/\": dial tcp 127.0.0.1:9200: connect: connection refused",
		},
		{
			name:     "lookback",
			expected: "failed to query indices: Get \"http://localhost:9200/jaeger-*?flat_settings=true&filter_path=*.aliases,*.settings\": dial tcp 127.0.0.1:9200: connect: connection refused",
		},
		{
			name:     "rollover",
			expected: "failed to create rollover: Post \"http://localhost:9200/jaeger-span-write/_rollover/\": dial tcp 127.0.0.1:9200: connect: connection refused",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testCommands(t, test.name, test.expected)
		})
	}
}

func testCommands(t *testing.T, action, connectionError string) {
	v := viper.New()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	cmd := Command(v, logger)
	cmd.SetArgs([]string{action, "http://localhost:9200"})
	var b bytes.Buffer
	cmd.SetOut(&b)
	err = cmd.Execute()
	require.EqualError(t, err, connectionError)
	cmd.SetArgs([]string{action})
	err = cmd.Execute()
	assert.EqualError(t, err, argumentsError)
}
