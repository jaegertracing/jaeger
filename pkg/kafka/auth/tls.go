// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"

	"github.com/Shopify/sarama"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

func setTLSConfiguration(config *tlscfg.Options, saramaConfig *sarama.Config, logger *zap.Logger) error {
	if config.Enabled {
		tlsConfig, err := config.Config(logger)
		if err != nil {
			return fmt.Errorf("error loading tls config: %w", err)
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}
	return nil
}
