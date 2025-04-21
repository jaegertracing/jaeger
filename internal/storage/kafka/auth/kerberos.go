// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"github.com/Shopify/sarama"
)

// KerberosConfig describes the configuration properties needed for Kerberos authentication with kafka consumer
type KerberosConfig struct {
	ServiceName     string `mapstructure:"service_name"`
	Realm           string `mapstructure:"realm"`
	UseKeyTab       bool   `mapstructure:"use_keytab"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password" json:"-"`
	ConfigPath      string `mapstructure:"config_file"`
	KeyTabPath      string `mapstructure:"keytab_file"`
	DisablePAFXFast bool   `mapstructure:"disable_pa_fx_fast"`
}

func setKerberosConfiguration(config *KerberosConfig, saramaConfig *sarama.Config) {
	saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeGSSAPI
	saramaConfig.Net.SASL.Enable = true
	if config.UseKeyTab {
		saramaConfig.Net.SASL.GSSAPI.KeyTabPath = config.KeyTabPath
		saramaConfig.Net.SASL.GSSAPI.AuthType = sarama.KRB5_KEYTAB_AUTH
	} else {
		saramaConfig.Net.SASL.GSSAPI.AuthType = sarama.KRB5_USER_AUTH
		saramaConfig.Net.SASL.GSSAPI.Password = config.Password
	}
	saramaConfig.Net.SASL.GSSAPI.KerberosConfigPath = config.ConfigPath
	saramaConfig.Net.SASL.GSSAPI.Username = config.Username
	saramaConfig.Net.SASL.GSSAPI.Realm = config.Realm
	saramaConfig.Net.SASL.GSSAPI.ServiceName = config.ServiceName
	saramaConfig.Net.SASL.GSSAPI.DisablePAFXFAST = config.DisablePAFXFast
}
