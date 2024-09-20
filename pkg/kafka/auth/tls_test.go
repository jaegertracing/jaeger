// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"
)

func TestSetTLSConfiguration(t *testing.T) {
	saramaConfig := sarama.NewConfig()
	tlsConfig := &configtls.ClientConfig{
		Insecure: false,
	}
	err := setTLSConfiguration(context.Background(), tlsConfig, saramaConfig)
	require.NoError(t, err)
	assert.True(t, saramaConfig.Net.TLS.Enable)
	assert.NotNil(t, saramaConfig.Net.TLS.Config)
}
