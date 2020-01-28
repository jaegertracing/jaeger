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
	"strings"

	"github.com/Shopify/sarama"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

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
}

// AuthenticationConfig describes the configuration properties needed authenticate with kafka cluster
type AuthenticationConfig struct {
	Authentication string
	Kerberos       KerberosConfig
	TLS            tlscfg.Options
	PlainText      PlainTextConfig
}

//SetConfiguration set configure authentication into sarama config structure
func (config *AuthenticationConfig) SetConfiguration(saramaConfig *sarama.Config) error {
	authentication := strings.ToLower(config.Authentication)
	if strings.Trim(authentication, " ") == "" {
		authentication = none
	}
	switch authentication {
	case none:
		return nil
	case kerberos:
		setKerberosConfiguration(&config.Kerberos, saramaConfig)
		return nil
	case tls:
		return setTLSConfiguration(&config.TLS, saramaConfig)
	case plaintext:
		setPlainTextConfiguration(&config.PlainText, saramaConfig)
		return nil
	default:
		return errors.Errorf("Unknown/Unsupported authentication method %s to kafka cluster.", config.Authentication)
	}
}

// InitFromViper loads authentication configuration from viper flags.
func (config *AuthenticationConfig) InitFromViper(configPrefix string, v *viper.Viper) {
	config.Authentication = v.GetString(configPrefix + suffixAuthentication)
	config.Kerberos.ServiceName = v.GetString(configPrefix + kerberosPrefix + suffixKerberosServiceName)
	config.Kerberos.Realm = v.GetString(configPrefix + kerberosPrefix + suffixKerberosRealm)
	config.Kerberos.UseKeyTab = v.GetBool(configPrefix + kerberosPrefix + suffixKerberosUseKeyTab)
	config.Kerberos.Username = v.GetString(configPrefix + kerberosPrefix + suffixKerberosUserName)
	config.Kerberos.Password = v.GetString(configPrefix + kerberosPrefix + suffixKerberosPassword)
	config.Kerberos.ConfigPath = v.GetString(configPrefix + kerberosPrefix + suffixKerberosConfig)
	config.Kerberos.KeyTabPath = v.GetString(configPrefix + kerberosPrefix + suffixKerberosKeyTab)

	var tlsClientConfig = tlscfg.ClientFlagsConfig{
		Prefix:         configPrefix,
		ShowEnabled:    true,
		ShowServerName: true,
	}

	config.TLS = tlsClientConfig.InitFromViper(v)

	config.PlainText.UserName = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextUserName)
	config.PlainText.Password = v.GetString(configPrefix + plainTextPrefix + suffixPlainTextPassword)
}
