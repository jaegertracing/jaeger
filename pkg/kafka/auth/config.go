// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

const (
	none      = "none"
	kerberos  = "kerberos"
	tls       = "tls"
	plaintext = "plaintext"
)

var authTypes = []string{
	none,
	kerberos,
	tls,
	plaintext,
}

// AuthenticationConfig describes the configuration properties needed authenticate with kafka cluster
type AuthenticationConfig struct {
	Authentication string                 `mapstructure:"type"`
	Kerberos       KerberosConfig         `mapstructure:"kerberos"`
	TLS            configtls.ClientConfig `mapstructure:"tls"`
	PlainText      PlainTextConfig        `mapstructure:"plaintext"`
}

// SetConfiguration set configure authentication into sarama config structure
func (config *AuthenticationConfig) SetConfiguration(saramaConfig *sarama.Config, logger *zap.Logger) error {
	authentication := strings.ToLower(config.Authentication)
	if strings.Trim(authentication, " ") == "" {
		authentication = none
	}
	if config.Authentication == tls {
		err := setTLSConfiguration(&config.TLS, saramaConfig, logger)
		if err != nil {
			return err
		}
	}
	switch authentication {
	case none:
		return nil
	case tls:
		return nil
	case kerberos:
		setKerberosConfiguration(&config.Kerberos, saramaConfig)
		return nil
	case plaintext:
		return setPlainTextConfiguration(&config.PlainText, saramaConfig)
	default:
		return fmt.Errorf("Unknown/Unsupported authentication method %s to kafka cluster", config.Authentication)
	}
}

// InitFromViper loads authentication configuration from viper flags.
func (config *AuthenticationConfig) InitFromViper(configPrefix string, v *viper.Viper) error {
	config.Authentication = v.GetString(configPrefix + suffixAuthentication)
	config.Kerberos.ServiceName = v.GetString(configPrefix + kerberosPrefix + suffixKerberosServiceName)
	config.Kerberos.Realm = v.GetString(configPrefix + kerberosPrefix + suffixKerberosRealm)
	config.Kerberos.UseKeyTab = v.GetBool(configPrefix + kerberosPrefix + suffixKerberosUseKeyTab)
	config.Kerberos.Username = v.GetString(configPrefix + kerberosPrefix + suffixKerberosUsername)
	config.Kerberos.Password = v.GetString(configPrefix + kerberosPrefix + suffixKerberosPassword)
	config.Kerberos.ConfigPath = v.GetString(configPrefix + kerberosPrefix + suffixKerberosConfig)
	config.Kerberos.KeyTabPath = v.GetString(configPrefix + kerberosPrefix + suffixKerberosKeyTab)
	config.Kerberos.DisablePAFXFast = v.GetBool(configPrefix + kerberosPrefix + suffixKerberosDisablePAFXFAST)

	tlsClientConfig := tlscfg.ClientFlagsConfig{
		Prefix: configPrefix,
	}

	opts, err := tlsClientConfig.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to process Kafka TLS options: %w", err)
	}
	if config.Authentication == tls {
		opts.Enabled = true
		config.TLS = opts.ToOtelClientConfig()
	}
	config.PlainText.Username = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextUsername)
	config.PlainText.Password = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextPassword)
	config.PlainText.Mechanism = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextMechanism)
	return nil
}
