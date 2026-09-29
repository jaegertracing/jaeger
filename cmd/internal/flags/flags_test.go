// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"fmt"
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

func TestParseJaegerTags(t *testing.T) {
	tags, err := ParseJaegerTags("")
	require.NoError(t, err)
	assert.Nil(t, tags)

	jaegerTags := fmt.Sprintf(
		"%s,%s,%s,%s,%s,%s",
		"key=value",
		"envVar1=${envKey1:defaultVal1}",
		"envVar2=${envKey2:defaultVal2}",
		"envVar3=${envKey3}",
		"envVar4=${envKey4}",
		"envVar5=${envVar5:}",
	)

	t.Setenv("envKey1", "envVal1")
	t.Setenv("envKey4", "envVal4")

	expectedTags := map[string]string{
		"key":     "value",
		"envVar1": "envVal1",
		"envVar2": "defaultVal2",
		"envVar4": "envVal4",
		"envVar5": "",
	}

	tags, err = ParseJaegerTags(jaegerTags)
	require.NoError(t, err)
	assert.Equal(t, expectedTags, tags)
}

func TestParseJaegerTagsError(t *testing.T) {
	_, err := ParseJaegerTags("no-equals-sign")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid Jaeger tag pair")
}
