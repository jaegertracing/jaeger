// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"flag"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func addFlags(flags *flag.FlagSet) {
	configPrefix := "kafka.auth"
	AddFlags(configPrefix, flags)
}

func Test_InitFromViper(t *testing.T) {
	configPrefix := "kafka.auth"
	v, command := config.Viperize(addFlags)
	command.ParseFlags([]string{
		"--kafka.auth.authentication=tls",
		"--kafka.auth.kerberos.service-name=kafka",
		"--kafka.auth.kerberos.realm=EXAMPLE.COM",
		"--kafka.auth.kerberos.use-keytab=true",
		"--kafka.auth.kerberos.username=user",
		"--kafka.auth.kerberos.password=password",
		"--kafka.auth.kerberos.config-file=/path/to/krb5.conf",
		"--kafka.auth.kerberos.keytab-file=/path/to/keytab",
		"--kafka.auth.kerberos.disable-fast-negotiation=true",
		"--kafka.auth.tls.enabled=false",
		"--kafka.auth.tls.ca=/not/allowed/if/tls/is/disabled",
		"--kafka.auth.plaintext.username=user",
		"--kafka.auth.plaintext.password=password",
		"--kafka.auth.plaintext.mechanism=SCRAM-SHA-256",
	})

	authConfig := &AuthenticationConfig{}
	err := authConfig.InitFromViper(configPrefix, v)
	require.ErrorContains(t, err, "kafka.auth.tls.* options cannot be used when kafka.auth.tls.enabled is false")

	command.ParseFlags([]string{
		"--kafka.auth.tls.enabled=true",
		"--kafka.auth.tls.ca=",
	}) // incrementally update authConfig
	require.NoError(t, authConfig.InitFromViper(configPrefix, v))

	expectedConfig := &AuthenticationConfig{
		Authentication: "tls",
		Kerberos: KerberosConfig{
			ServiceName:     "kafka",
			Realm:           "EXAMPLE.COM",
			UseKeyTab:       true,
			Username:        "user",
			Password:        "password",
			ConfigPath:      "/path/to/krb5.conf",
			KeyTabPath:      "/path/to/keytab",
			DisablePAFXFast: true,
		},
		TLS: configtls.ClientConfig{
			Config: configtls.Config{
				IncludeSystemCACertsPool: true,
			},
		},
		PlainText: PlainTextConfig{
			Username:  "user",
			Password:  "password",
			Mechanism: "SCRAM-SHA-256",
		},
	}
	assert.Equal(t, expectedConfig, authConfig)
}

// Test plaintext with different mechanisms
func testPlaintext(v *viper.Viper, t *testing.T, configPrefix string, logger *zap.Logger, mechanism string, saramaConfig *sarama.Config) {
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextMechanism, mechanism)
	authConfig := &AuthenticationConfig{}
	err := authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	require.NoError(t, authConfig.SetConfiguration(saramaConfig, logger))
}

func TestSetConfiguration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	saramaConfig := sarama.NewConfig()
	configPrefix := "kafka.auth"
	v, command := config.Viperize(addFlags)

	// Table-driven test cases
	tests := []struct {
		name                string
		authType            string
		expectedError       string
		plainTextMechanisms []string
	}{
		{
			name:          "Invalid authentication method",
			authType:      "fail",
			expectedError: "Unknown/Unsupported authentication method fail to kafka cluster",
		},
		{
			name:          "Kerberos authentication",
			authType:      "kerberos",
			expectedError: "",
		},
		{
			name:                "Plaintext authentication with SCRAM-SHA-256",
			authType:            "plaintext",
			expectedError:       "",
			plainTextMechanisms: []string{"SCRAM-SHA-256"},
		},
		{
			name:                "Plaintext authentication with SCRAM-SHA-512",
			authType:            "plaintext",
			expectedError:       "",
			plainTextMechanisms: []string{"SCRAM-SHA-512"},
		},
		{
			name:                "Plaintext authentication with PLAIN",
			authType:            "plaintext",
			expectedError:       "",
			plainTextMechanisms: []string{"PLAIN"},
		},
		{
			name:          "No authentication",
			authType:      " ",
			expectedError: "",
		},
		{
			name:          "TLS authentication",
			authType:      "tls",
			expectedError: "",
		},
		{
			name:          "TLS authentication with invalid cipher suite",
			authType:      "tls",
			expectedError: "error loading tls config: failed to load TLS config: invalid TLS cipher suite: \"fail\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command.ParseFlags([]string{
				"--kafka.auth.authentication=" + tt.authType,
			})
			authConfig := &AuthenticationConfig{}
			err := authConfig.InitFromViper(configPrefix, v)
			require.NoError(t, err)

			if tt.authType == "tls" && tt.expectedError != "" {
				authConfig.TLS.CipherSuites = []string{"fail"}
			}

			if len(tt.plainTextMechanisms) > 0 {
				for _, mechanism := range tt.plainTextMechanisms {
					testPlaintext(v, t, configPrefix, logger, mechanism, saramaConfig)
				}
			} else {
				err = authConfig.SetConfiguration(saramaConfig, logger)
				if tt.expectedError != "" {
					require.EqualError(t, err, tt.expectedError)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}
