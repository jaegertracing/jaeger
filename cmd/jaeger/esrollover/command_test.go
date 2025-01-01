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
	argumentsError    = "accepts 1 arg(s), received 0"
	connectionRefused = "connection refused"
)

func TestCommand(t *testing.T) {
	v := viper.New()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	cmd := Command(v, logger)
	assert.NotNil(t, cmd)
	assert.Equal(t, "es-rollover", cmd.Use)
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestAllCommands(t *testing.T) {
	tests := []string{"init", "lookback", "rollover"}
	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			testCommands(t, test)
		})
	}
}

func testCommands(t *testing.T, action string) {
	v := viper.New()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	cmd := Command(v, logger)
	cmd.SetArgs([]string{action, "http://localhost:9200"})
	var b bytes.Buffer
	cmd.SetOut(&b)
	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), connectionRefused)
	cmd.SetArgs([]string{action})
	err = cmd.Execute()
	assert.EqualError(t, err, argumentsError)
}
