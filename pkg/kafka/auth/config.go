// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"fmt"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
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
	Authentication string          `mapstructure:"type"`
	Kerberos       KerberosConfig  `mapstructure:"kerberos"`
	TLS            tlscfg.Options  `mapstructure:"tls"`
	PlainText      PlainTextConfig `mapstructure:"plaintext"`
}

// SetConfiguration set configure authentication into sarama config structure
func (config *AuthenticationConfig) SetConfiguration(saramaConfig *sarama.Config, logger *zap.Logger) error {
	authentication := strings.ToLower(config.Authentication)
	if strings.Trim(authentication, " ") == "" {
		authentication = none
	}
	if config.Authentication == tls || config.TLS.Enabled {
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

	var err error
	config.TLS, err = tlsClientConfig.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to process Kafka TLS options: %w", err)
	}
	if config.Authentication == tls {
		config.TLS.Enabled = true
	}

	config.PlainText.Username = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextUsername)
	config.PlainText.Password = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextPassword)
	config.PlainText.Mechanism = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextMechanism)
	return nil
}
