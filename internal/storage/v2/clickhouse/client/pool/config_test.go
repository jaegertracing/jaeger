// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package pool

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestDefaultConfig(t *testing.T) {
	expected := Configuration{
		ClientConfig: ClientConfig{
			Address:  "127.0.0.1:9000",
			Database: "default",
			Username: "default",
			Password: "default",
		},
		PoolConfig: Config{
			MaxConnLifetime:   time.Hour,
			MaxConnIdleTime:   time.Minute * 30,
			MinConns:          int32(runtime.NumCPU()),
			MaxConns:          int32(runtime.NumCPU() * 2),
			HealthCheckPeriod: time.Minute,
		},
	}

	actual := DefaultConfig()
	assert.NotNil(t, actual)
	assert.Equal(t, expected, actual)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
