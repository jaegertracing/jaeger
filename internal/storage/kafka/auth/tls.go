// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"fmt"

	"github.com/Shopify/sarama"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
)

func setTLSConfiguration(config *configtls.ClientConfig, saramaConfig *sarama.Config, logger *zap.Logger) error {
    tlsConfig, err := config.LoadTLSConfig(context.Background())
    if err != nil {
        return fmt.Errorf("error loading tls config: %w", err)
    }
    
    saramaConfig.Net.TLS.Enable = true
    saramaConfig.Net.TLS.Config = tlsConfig
    logger.Info("TLS configuration enabled for Kafka client", 
        zap.Bool("skip_verify", config.InsecureSkipVerify),
        zap.String("ca_file", config.CAFile),
        zap.Bool("system_ca_enabled", config.Config.IncludeSystemCACertsPool))
    return nil
}
