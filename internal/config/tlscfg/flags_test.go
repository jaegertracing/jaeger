// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tlscfg

import (
	"flag"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/config"
)

func TestClientFlags(t *testing.T) {
	cmdLine := []string{
		"--prefix.tls.ca=ca-file",
		"--prefix.tls.cert=cert-file",
		"--prefix.tls.key=key-file",
		"--prefix.tls.server-name=HAL1",
		"--prefix.tls.skip-host-verify=true",
	}

	tests := []struct {
		option string
	}{
		{
			option: "--prefix.tls.enabled=true",
		},
	}

	for _, test := range tests {
		t.Run(test.option, func(t *testing.T) {
			v := viper.New()
			command := cobra.Command{}
			flagSet := &flag.FlagSet{}
			flagCfg := ClientFlagsConfig{
				Prefix: "prefix",
			}
			flagCfg.AddFlags(flagSet)
			command.PersistentFlags().AddGoFlagSet(flagSet)
			v.BindPFlags(command.PersistentFlags())

			err := command.ParseFlags(append(cmdLine, test.option))
			require.NoError(t, err)
			clientCfg, err := flagCfg.InitFromViper(v)
			require.NoError(t, err)

			expectedCfg := configtls.ClientConfig{
				Config: configtls.Config{
					CAFile:                   "ca-file",
					CertFile:                 "cert-file",
					KeyFile:                  "key-file",
					IncludeSystemCACertsPool: false,
					CipherSuites:             nil,
					ReloadInterval:           0,
				},
				Insecure:           false,
				InsecureSkipVerify: true,
				ServerName:         "HAL1",
			}

			assert.Equal(t, expectedCfg, clientCfg)
		})
	}
}

func TestServerFlags(t *testing.T) {
	cmdLine := []string{
		"##placeholder##", // replaced in each test below
		"--prefix.tls.enabled=true",
		"--prefix.tls.cert=cert-file",
		"--prefix.tls.key=key-file",
		"--prefix.tls.cipher-suites=TLS_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
		"--prefix.tls.min-version=1.2",
		"--prefix.tls.max-version=1.3",
	}

	tests := []struct {
		option string
		file   string
	}{
		{
			option: "--prefix.tls.client-ca=client-ca-file",
			file:   "client-ca-file",
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			v := viper.New()
			command := cobra.Command{}
			flagSet := &flag.FlagSet{}
			flagCfg := ServerFlagsConfig{
				Prefix: "prefix",
			}
			flagCfg.AddFlags(flagSet)
			command.PersistentFlags().AddGoFlagSet(flagSet)
			v.BindPFlags(command.PersistentFlags())

			cmdLine[0] = test.option
			err := command.ParseFlags(cmdLine)
			require.NoError(t, err)
			serverConfig, err := flagCfg.InitFromViper(v)
			require.NoError(t, err)

			expectedConfig := configtls.ServerConfig{
				ClientCAFile: "client-ca-file",
				Config: configtls.Config{
					CertFile:     "cert-file",
					KeyFile:      "key-file",
					MinVersion:   "1.2",
					MaxVersion:   "1.3",
					CipherSuites: []string{"TLS_AES_256_GCM_SHA384", "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA"},
				},
			}

			assert.Equal(t, expectedConfig, *serverConfig)
		})
	}
}

func TestServerCertReloadInterval(t *testing.T) {
	cfg := ServerFlagsConfig{
		Prefix: "prefix",
	}
	v, command := config.Viperize(cfg.AddFlags)
	err := command.ParseFlags([]string{
		"--prefix.tls.enabled=true",
		"--prefix.tls.reload-interval=24h",
	})
	require.NoError(t, err)
	tlscfg, err := cfg.InitFromViper(v)
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, tlscfg.ReloadInterval)
}

// TestFailedTLSFlags verifies that TLS options cannot be used when tls.enabled=false
func TestFailedTLSFlags(t *testing.T) {
	clientTests := []string{
		".ca=blah",
		".cert=blah",
		".key=blah",
		".server-name=blah",
		".skip-host-verify=true",
	}
	serverTests := []string{
		".cert=blah",
		".key=blah",
		".client-ca=blah",
		".cipher-suites=blah",
		".min-version=1.1",
		".max-version=1.3",
	}

	allTests := []struct {
		side  string
		tests []string
	}{
		{side: "client", tests: clientTests},
		{side: "server", tests: serverTests},
	}

	for _, metaTest := range allTests {
		t.Run(metaTest.side, func(t *testing.T) {
			for _, test := range metaTest.tests {
				t.Run(test, func(t *testing.T) {
					var (
						addFlags      func(*flag.FlagSet)
						initFromViper func(*viper.Viper) (any, error)
					)

					if metaTest.side == "client" {
						clientConfig := &ClientFlagsConfig{Prefix: "prefix"}
						addFlags = clientConfig.AddFlags
						initFromViper = func(v *viper.Viper) (any, error) {
							return clientConfig.InitFromViper(v)
						}
					} else {
						serverConfig := &ServerFlagsConfig{Prefix: "prefix"}
						addFlags = serverConfig.AddFlags
						initFromViper = func(v *viper.Viper) (any, error) {
							return serverConfig.InitFromViper(v)
						}
					}

					v, command := config.Viperize(addFlags)

					cmdLine := []string{
						"--prefix.tls.enabled=true",
						"--prefix.tls" + test,
					}
					err := command.ParseFlags(cmdLine)
					require.NoError(t, err)

					result, err := initFromViper(v)
					require.NoError(t, err)

					switch metaTest.side {
					case "client":
						require.IsType(t, configtls.ClientConfig{}, result, "result should be of type configtls.ClientConfig")
					case "server":
						require.IsType(t, &configtls.ServerConfig{}, result, "result should be of type *configtls.ServerConfig")
					}

					cmdLine[0] = "--prefix.tls.enabled=false"
					err = command.ParseFlags(cmdLine)
					require.NoError(t, err)

					_, err = initFromViper(v)
					require.Error(t, err)
					require.EqualError(t, err, "prefix.tls.* options cannot be used when prefix.tls.enabled is false")
				})
			}
		})
	}
}
