package auth

import (
	"testing"
	"github.com/spf13/viper"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	//"github.com/davecgh/go-spew/spew"
)


func Test_InitFromViper(t *testing.T) {
	//kebrose_test
	v := viper.New()
	configPrefix := "kafka.auth."
	v.Set(configPrefix + suffixAuthentication, "kerberos")
	v.Set(configPrefix + kerberosPrefix + suffixKerberosServiceName, "kafka")
	v.Set(configPrefix + kerberosPrefix + suffixKerberosRealm, "EXAMPLE.COM")
	v.Set(configPrefix + kerberosPrefix + suffixKerberosUseKeyTab, true)
	v.Set(configPrefix + kerberosPrefix + suffixKerberosUsername, "user")
	v.Set(configPrefix + kerberosPrefix + suffixKerberosPassword, "password")
	v.Set(configPrefix + kerberosPrefix + suffixKerberosConfig, "/path/to/krb5.conf")
	v.Set(configPrefix + kerberosPrefix + suffixKerberosKeyTab, "/path/to/keytab")
	v.Set(configPrefix + kerberosPrefix + suffixKerberosDisablePAFXFAST, true)
	v.Set(configPrefix + plainTextPrefix + suffixPlainTextUsername, "user")
	v.Set(configPrefix + plainTextPrefix + suffixPlainTextPassword, "password")
	v.Set(configPrefix + plainTextPrefix + suffixPlainTextMechanism, "SCRAM-SHA-256")

	authConfig := &AuthenticationConfig{}
	err := authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	expectedConfig := &AuthenticationConfig{
		Authentication: "kerberos",
		Kerberos: KerberosConfig{
			ServiceName:      "kafka",
			Realm:            "EXAMPLE.COM",
			UseKeyTab:        true,
			Username:         "user",
			Password:         "password",
			ConfigPath:       "/path/to/krb5.conf",
			KeyTabPath:       "/path/to/keytab",
			DisablePAFXFast:  true,
		},
		TLS: tlscfg.Options{},
		PlainText: PlainTextConfig{
			Username:   "user",
			Password:   "password",
			Mechanism:  "SCRAM-SHA-256",
		},
	}
	assert.Equal(t, expectedConfig, authConfig)
	//no authentication test
	v.Set(configPrefix + suffixAuthentication, "")
	authConfig = &AuthenticationConfig{}
	err = authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)

	expectedConfig.Authentication = ""
	assert.Equal(t, expectedConfig, authConfig)

	//plain text test
	v.Set(configPrefix + suffixAuthentication, "plaintext")
	authConfig = &AuthenticationConfig{}
	err = authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)
	expectedConfig.Authentication = "plaintext"
	assert.Equal(t, expectedConfig, authConfig)
	//tls test
	v.Set(configPrefix + suffixAuthentication, "tls")
	authConfig = &AuthenticationConfig{}
	err = authConfig.InitFromViper(configPrefix, v)
	require.NoError(t, err)

	expectedConfig.Authentication = "tls"
	tlsClientConfig := tlscfg.ClientFlagsConfig{
		Prefix: configPrefix,
	}
	expectedConfig.TLS, _ = tlsClientConfig.InitFromViper(v)
	expectedConfig.TLS.Enabled = true
	assert.Equal(t, expectedConfig, authConfig)
}