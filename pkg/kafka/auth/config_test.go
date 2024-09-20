// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"flag"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

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
		"--kafka.auth.plaintext.username=user",
		"--kafka.auth.plaintext.password=password",
		"--kafka.auth.plaintext.mechanism=SCRAM-SHA-256",
		"--kafka.auth.tls.ca=failing",
	})

	authConfig := &AuthenticationConfig{}
	err := authConfig.InitFromViper(configPrefix, v)
	require.EqualError(t, err, "failed to process Kafka TLS options: kafka.auth.tls.* options cannot be used when kafka.auth.tls.enabled is false")

	command.ParseFlags([]string{"--kafka.auth.tls.ca="})
	v.BindPFlags(command.Flags())
	err = authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)

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
			Insecure: false,
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
func testPlaintext(ctx context.Context, v *viper.Viper, t *testing.T, configPrefix string, mechanism string, saramaConfig *sarama.Config) {
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextMechanism, mechanism)
	authConfig := &AuthenticationConfig{}
	err := authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	require.NoError(t, authConfig.SetConfiguration(ctx, saramaConfig))
}

func TestSetConfiguration(t *testing.T) {
	saramaConfig := sarama.NewConfig()
	configPrefix := "kafka.auth"
	v, command := config.Viperize(addFlags)

	// Table-driven test cases
	tests := []struct {
		name                string
		authType            string
		expectedError       error
		plainTextMechanisms []string
	}{
		{
			name:          "Invalid authentication method",
			authType:      "fail",
			expectedError: ErrUnsupportedAuthMethod,
		},
		{
			name:     "Kerberos authentication",
			authType: "kerberos",
		},
		{
			name:                "Plaintext authentication with SCRAM-SHA-256",
			authType:            "plaintext",
			plainTextMechanisms: []string{"SCRAM-SHA-256"},
		},
		{
			name:                "Plaintext authentication with SCRAM-SHA-512",
			authType:            "plaintext",
			plainTextMechanisms: []string{"SCRAM-SHA-512"},
		},
		{
			name:                "Plaintext authentication with PLAIN",
			authType:            "plaintext",
			plainTextMechanisms: []string{"PLAIN"},
		},
		{
			name:     "No authentication",
			authType: " ",
		},
		{
			name:     "TLS authentication",
			authType: "tls",
		},
		{
			name:          "TLS authentication with invalid cipher suite",
			authType:      "tls",
			expectedError: ErrLoadingTLSConfig,
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

			if tt.authType == "tls" && tt.expectedError != nil {
				authConfig.TLS.CipherSuites = []string{"fail"}
			}

			if len(tt.plainTextMechanisms) > 0 {
				for _, mechanism := range tt.plainTextMechanisms {
					testPlaintext(context.Background(), v, t, configPrefix, mechanism, saramaConfig)
				}
			} else {
				err = authConfig.SetConfiguration(context.Background(), saramaConfig)
				if tt.expectedError != nil {
					require.ErrorIs(t, err, tt.expectedError)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}
