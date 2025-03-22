// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap/zaptest"
)

func TestSetTLSConfiguration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	saramaConfig := sarama.NewConfig()
	tlsConfig := &configtls.ClientConfig{}
	err := setTLSConfiguration(tlsConfig, saramaConfig, logger)
	require.NoError(t, err)
	assert.True(t, saramaConfig.Net.TLS.Enable)
	assert.NotNil(t, saramaConfig.Net.TLS.Config)
}
