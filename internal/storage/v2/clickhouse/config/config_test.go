// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	excepted := Configuration{
		ClickhouseGoCfg: DefaultClickhouseGoConfig(),
		ChGoConfig:      DefaultChGoConfig(),
		CreateSchema:    true,
	}

	actual := DefaultConfiguration()
	assert.Equal(t, excepted, actual)
}

func TestDefaultClickhouseGoCfg(t *testing.T) {
	expected := ClickhouseGoConfig{
		Address:  []string{"127.0.0.1:9000"},
		Database: "default",
		Username: "default",
		Password: "default",
	}
	actual := DefaultClickhouseGoConfig()
	assert.NotNil(t, actual)
	assert.Equal(t, expected, actual)
}

func TestDefaultChGoConfig(t *testing.T) {
	expected := ChGoConfig{
		ClientConfig: ClientConfig{
			Address:  "127.0.0.1:9000",
			Database: "default",
			Username: "default",
			Password: "default",
		},
		PoolConfig: PoolConfig{
			MaxConnLifetime:   time.Hour,
			MaxConnIdleTime:   time.Minute * 30,
			MinConns:          int32(runtime.NumCPU()),
			MaxConns:          int32(runtime.NumCPU() * 2),
			HealthCheckPeriod: time.Minute,
		},
	}

	actual := DefaultChGoConfig()
	assert.NotNil(t, actual)
	assert.Equal(t, expected, actual)
}
