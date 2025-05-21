// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestDefaultConfig(t *testing.T) {
	excepted := Config{
		Servers:  []string{defaultServer},
		Database: defaultDatabase,
		Auth: Authentication{
			Username: defaultUsername,
			Password: defaultPassword,
		},
		TLS: configtls.NewDefaultClientConfig(),
	}

	actual := DefaultConfiguration()
	assert.Equal(t, excepted, actual)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
