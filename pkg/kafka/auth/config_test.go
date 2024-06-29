// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"flag"
	"strings"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

func Test_InitFromViper(t *testing.T) {
	configPrefix := "kafka.auth"
	flagSet := flag.NewFlagSet("flags", flag.ContinueOnError)
	AddFlags(configPrefix, flagSet)
	command := &cobra.Command{}
	command.Flags().AddGoFlagSet(flagSet)
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
		"--kafka.auth.tls.enabled=true",
		"--kafka.auth.plaintext.username=user",
		"--kafka.auth.plaintext.password=password",
		"--kafka.auth.plaintext.mechanism=SCRAM-SHA-256",
	})
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.BindPFlags(command.Flags())
	authConfig := &AuthenticationConfig{}
	err := authConfig.InitFromViper(configPrefix, v)
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
		TLS: tlscfg.Options{
			Enabled: true,
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
	logger, _ := zap.NewDevelopment()
	saramaConfig := sarama.NewConfig()
	configPrefix := "kafka.auth"
	flagSet := flag.NewFlagSet("flags", flag.ContinueOnError)
	AddFlags(configPrefix, flagSet)
	command := &cobra.Command{}
	command.Flags().AddGoFlagSet(flagSet)
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.BindPFlags(command.Flags())

	authConfig := &AuthenticationConfig{}

	// Helper function to parse flags and initialize authConfig
	parseFlagsAndInit := func(authType string) {
		command.ParseFlags([]string{
			"--kafka.auth.authentication=" + authType,
			"--kafka.auth.kerberos.service-name=kafka",
			"--kafka.auth.kerberos.realm=EXAMPLE.COM",
			"--kafka.auth.kerberos.use-keytab=true",
			"--kafka.auth.kerberos.username=user",
			"--kafka.auth.kerberos.password=password",
			"--kafka.auth.kerberos.config-file=/path/to/krb5.conf",
			"--kafka.auth.kerberos.keytab-file=/path/to/keytab",
			"--kafka.auth.kerberos.disable-fast-negotiation=true",
			"--kafka.auth.tls.enabled=true",
			"--kafka.auth.plaintext.username=user",
			"--kafka.auth.plaintext.password=password",
			"--kafka.auth.plaintext.mechanism=SCRAM-SHA-256",
		})
		authConfig = &AuthenticationConfig{}
		err := authConfig.InitFromViper(configPrefix, v)
		require.NoError(t, err)
	}

	// Test with invalid authentication method
	parseFlagsAndInit("fail")
	require.Error(t, authConfig.SetConfiguration(saramaConfig, logger), "Unknown/Unsupported authentication method fail to kafka cluster")

	// Test with kerberos
	parseFlagsAndInit("kerberos")
	require.NoError(t, authConfig.SetConfiguration(saramaConfig, logger))

	// Test all plaintext options
	parseFlagsAndInit("plaintext")
	testPlaintext(v, t, configPrefix, logger, "SCRAM-SHA-256", saramaConfig)
	testPlaintext(v, t, configPrefix, logger, "SCRAM-SHA-512", saramaConfig)
	testPlaintext(v, t, configPrefix, logger, "PLAIN", saramaConfig)

	// Test with tls
	parseFlagsAndInit("tls")
	require.NoError(t, authConfig.SetConfiguration(saramaConfig, logger))

	// Test with no authentication
	parseFlagsAndInit(" ")
	require.NoError(t, authConfig.SetConfiguration(saramaConfig, logger))
}
