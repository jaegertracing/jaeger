// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"

	"github.com/Shopify/sarama"
	"go.opentelemetry.io/collector/config/configtls"
)

var ErrLoadingTLSConfig = errors.New("error loading tls config")

func setTLSConfiguration(ctx context.Context, config *configtls.ClientConfig, saramaConfig *sarama.Config) error {
	if !config.Insecure {
		tlsConfig, err := config.LoadTLSConfig(ctx)
		if err != nil {
			return errors.Join(ErrLoadingTLSConfig, err)
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}
	return nil
}
