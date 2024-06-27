// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

func Test_InitFromViper(t *testing.T) {
	v := viper.New()
	configPrefix := "kafka.auth."
	v.Set(configPrefix+suffixAuthentication, "tls")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosServiceName, "kafka")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosRealm, "EXAMPLE.COM")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosUseKeyTab, true)
	v.Set(configPrefix+kerberosPrefix+suffixKerberosUsername, "user")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosPassword, "password")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosConfig, "/path/to/krb5.conf")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosKeyTab, "/path/to/keytab")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosDisablePAFXFAST, true)
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextUsername, "user")
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextPassword, "password")
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextMechanism, "SCRAM-SHA-256")

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

func TestSetConfiguration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	saramaConfig := sarama.NewConfig()
	v := viper.New()
	configPrefix := "kafka.auth."
	v.Set(configPrefix+suffixAuthentication, "kerberos")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosServiceName, "kafka")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosRealm, "EXAMPLE.COM")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosUseKeyTab, true)
	v.Set(configPrefix+kerberosPrefix+suffixKerberosUsername, "user")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosPassword, "password")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosConfig, "/path/to/krb5.conf")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosKeyTab, "/path/to/keytab")
	v.Set(configPrefix+kerberosPrefix+suffixKerberosDisablePAFXFAST, true)
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextUsername, "user")
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextPassword, "password")
	v.Set(configPrefix+plainTextPrefix+suffixPlainTextMechanism, "SCRAM-SHA-256")

	authConfig := &AuthenticationConfig{}
	err := authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	require.NoError(t, authConfig.SetConfiguration(saramaConfig, logger))
	// plaintest test
	v.Set(configPrefix+suffixAuthentication, "plaintext")
	authConfig = &AuthenticationConfig{}
	err = authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	require.NoError(t, authConfig.SetConfiguration(saramaConfig, logger))
	// none value
	v.Set(configPrefix+suffixAuthentication, " ")
	authConfig = &AuthenticationConfig{}
	err = authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	require.NoError(t, authConfig.SetConfiguration(saramaConfig, logger))
	// tls value
	// v.Set(configPrefix + suffixAuthentication, "tls")
	// authConfig = &AuthenticationConfig{}
	// err = authConfig.InitFromViper(configPrefix, v)
	// require.NoError(t, err)
	// _,err=authConfig.TLS.Config(zap.NewNop())
	// // require.NoError(t, err)
	// // require.NoError(t,authConfig.SetConfiguration(saramaConfig, logger))
	// default value
	v.Set(configPrefix+suffixAuthentication, "error")
	authConfig = &AuthenticationConfig{}
	err = authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	expected_error := "Unknown/Unsupported authentication method error to kafka cluster"
	require.Error(t, authConfig.SetConfiguration(saramaConfig, logger), expected_error)
}
