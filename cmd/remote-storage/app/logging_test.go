// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/config"
)

func TestConfigFile(t *testing.T) {
	v, cmd := config.Viperize(AddConfigFileFlag)
	assert.Empty(t, ConfigFile(v))
	require.NoError(t, cmd.ParseFlags([]string{"--config-file=/tmp/config.yaml"}))
	assert.Equal(t, "/tmp/config.yaml", ConfigFile(v))
}

func TestLoggingConfigNewLogger(t *testing.T) {
	for _, encoding := range []string{"json", "console"} {
		logger, err := LoggingConfig{Level: "debug", Encoding: encoding}.NewLogger(zap.NewProductionConfig())
		require.NoError(t, err)
		assert.True(t, logger.Core().Enabled(zap.DebugLevel))
	}
	_, err := LoggingConfig{Level: "nope", Encoding: "json"}.NewLogger(zap.NewProductionConfig())
	require.Error(t, err)
}
