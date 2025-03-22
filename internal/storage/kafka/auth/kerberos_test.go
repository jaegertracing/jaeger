// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
)

func TestSetKerberosConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config KerberosConfig
	}{
		{
			name: "With KeyTab",
			config: KerberosConfig{
				ServiceName:     "service",
				Realm:           "realm",
				UseKeyTab:       true,
				Username:        "username",
				Password:        "password",
				ConfigPath:      "/path/to/config",
				KeyTabPath:      "/path/to/keytab",
				DisablePAFXFast: true,
			},
		},
		{
			name: "Without KeyTab",
			config: KerberosConfig{
				ServiceName:     "service",
				Realm:           "realm",
				UseKeyTab:       false,
				Username:        "username",
				Password:        "password",
				ConfigPath:      "/path/to/config",
				DisablePAFXFast: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saramaConfig := sarama.NewConfig()

			setKerberosConfiguration(&tt.config, saramaConfig)

			assert.Equal(t, sarama.SASLMechanism("GSSAPI"), saramaConfig.Net.SASL.Mechanism)
			assert.True(t, saramaConfig.Net.SASL.Enable)
			assert.Equal(t, tt.config.Username, saramaConfig.Net.SASL.GSSAPI.Username)
			assert.Equal(t, tt.config.Realm, saramaConfig.Net.SASL.GSSAPI.Realm)
			assert.Equal(t, tt.config.ServiceName, saramaConfig.Net.SASL.GSSAPI.ServiceName)
			assert.Equal(t, tt.config.DisablePAFXFast, saramaConfig.Net.SASL.GSSAPI.DisablePAFXFAST)
			assert.Equal(t, tt.config.ConfigPath, saramaConfig.Net.SASL.GSSAPI.KerberosConfigPath)

			if tt.config.UseKeyTab {
				assert.Equal(t, tt.config.KeyTabPath, saramaConfig.Net.SASL.GSSAPI.KeyTabPath)
				assert.Equal(t, sarama.KRB5_KEYTAB_AUTH, saramaConfig.Net.SASL.GSSAPI.AuthType)
			} else {
				assert.Equal(t, tt.config.Password, saramaConfig.Net.SASL.GSSAPI.Password)
				assert.Equal(t, sarama.KRB5_USER_AUTH, saramaConfig.Net.SASL.GSSAPI.AuthType)
			}
		})
	}
}
