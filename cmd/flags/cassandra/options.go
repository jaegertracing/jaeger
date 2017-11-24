// Copyright (c) 2017 Uber Technologies, Inc.
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

package cassandra

import (
	"flag"
	"strings"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/cassandra/config"
)

const (
	suffixConnPerHost      = ".connections-per-host"
	suffixMaxRetryAttempts = ".max-retry-attempts"
	suffixTimeout          = ".timeout"
	suffixServers          = ".servers"
	suffixPort             = ".port"
	suffixKeyspace         = ".keyspace"
	suffixProtoVer         = ".proto-version"
	suffixSocketKeepAlive  = ".socket-keep-alive"
	suffixUsername         = ".username"
	suffixPassword         = ".password"
	suffixTLS              = ".tls"
	suffixCert             = ".tls.cert"
	suffixKey              = ".tls.key"
	suffixCA               = ".tls.ca"
	suffixServerName       = ".tls.server-name"
	suffixVerifyHost       = ".tls.verify-host"
)

// TODO this should be moved next to config.Configuration struct (maybe ./flags package)

// Options contains various type of Cassandra configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	primary *namespaceConfig

	others map[string]*namespaceConfig
}

// the Servers field in config.Configuration is a list, which we cannot represent with flags.
// This struct adds a plain string field that can be bound to flags and is then parsed when
// preparing the actual config.Configuration.
type namespaceConfig struct {
	config.Configuration
	servers   string
	namespace string
}

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string, otherNamespaces ...string) *Options {
	// TODO all default values should be defined via cobra flags
	options := &Options{
		primary: &namespaceConfig{
			Configuration: config.Configuration{
				TLS: config.TLS{
					Enabled:                false,
					EnableHostVerification: true,
				},
				MaxRetryAttempts:   3,
				Keyspace:           "jaeger_v1_local",
				ProtoVersion:       4,
				ConnectionsPerHost: 2,
			},
			servers:   "127.0.0.1",
			namespace: primaryNamespace,
		},
		others: make(map[string]*namespaceConfig, len(otherNamespaces)),
	}

	for _, namespace := range otherNamespaces {
		options.others[namespace] = &namespaceConfig{namespace: namespace}
	}

	return options
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, opt.primary)
	for _, cfg := range opt.others {
		addFlags(flagSet, cfg)
	}
}

func addFlags(flagSet *flag.FlagSet, nsConfig *namespaceConfig) {
	flagSet.Int(
		nsConfig.namespace+suffixConnPerHost,
		nsConfig.ConnectionsPerHost,
		"The number of Cassandra connections from a single backend instance")
	flagSet.Int(
		nsConfig.namespace+suffixMaxRetryAttempts,
		nsConfig.MaxRetryAttempts,
		"The number of attempts when reading from Cassandra")
	flagSet.Duration(
		nsConfig.namespace+suffixTimeout,
		nsConfig.Timeout,
		"Timeout used for queries")
	flagSet.String(
		nsConfig.namespace+suffixServers,
		nsConfig.servers,
		"The comma-separated list of Cassandra servers")
	flagSet.Int(
		nsConfig.namespace+suffixPort,
		nsConfig.Port,
		"The port for cassandra")
	flagSet.String(
		nsConfig.namespace+suffixKeyspace,
		nsConfig.Keyspace,
		"The Cassandra keyspace for Jaeger data")
	flagSet.Int(
		nsConfig.namespace+suffixProtoVer,
		nsConfig.ProtoVersion,
		"The Cassandra protocol version")
	flagSet.Duration(
		nsConfig.namespace+suffixSocketKeepAlive,
		nsConfig.SocketKeepAlive,
		"Cassandra's keepalive period to use, enabled if > 0")
	flagSet.String(
		nsConfig.namespace+suffixUsername,
		nsConfig.Authenticator.Basic.Username,
		"Username for password authentication for Cassandra")
	flagSet.String(
		nsConfig.namespace+suffixPassword,
		nsConfig.Authenticator.Basic.Password,
		"Password for password authentication for Cassandra")
	flagSet.Bool(
		nsConfig.namespace+suffixTLS,
		nsConfig.TLS.Enabled,
		"Enable TLS")
	flagSet.String(
		nsConfig.namespace+suffixCert,
		nsConfig.TLS.CertPath,
		"Path to TLS certificate file")
	flagSet.String(
		nsConfig.namespace+suffixKey,
		nsConfig.TLS.KeyPath,
		"Path to TLS key file")
	flagSet.String(
		nsConfig.namespace+suffixCA,
		nsConfig.TLS.CaPath,
		"Path to TLS CA file")
	flagSet.String(
		nsConfig.namespace+suffixServerName,
		nsConfig.TLS.ServerName,
		"Override the TLS server name")
	flagSet.Bool(
		nsConfig.namespace+suffixVerifyHost,
		nsConfig.TLS.EnableHostVerification,
		"Enable (or disable) host key verification")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	initFromViper(opt.primary, v)
	for _, cfg := range opt.others {
		initFromViper(cfg, v)
	}
}

func initFromViper(cfg *namespaceConfig, v *viper.Viper) {
	cfg.ConnectionsPerHost = v.GetInt(cfg.namespace + suffixConnPerHost)
	cfg.MaxRetryAttempts = v.GetInt(cfg.namespace + suffixMaxRetryAttempts)
	cfg.Timeout = v.GetDuration(cfg.namespace + suffixTimeout)
	cfg.servers = v.GetString(cfg.namespace + suffixServers)
	cfg.Port = v.GetInt(cfg.namespace + suffixPort)
	cfg.Keyspace = v.GetString(cfg.namespace + suffixKeyspace)
	cfg.ProtoVersion = v.GetInt(cfg.namespace + suffixProtoVer)
	cfg.SocketKeepAlive = v.GetDuration(cfg.namespace + suffixSocketKeepAlive)
	cfg.Authenticator.Basic.Username = v.GetString(cfg.namespace + suffixUsername)
	cfg.Authenticator.Basic.Password = v.GetString(cfg.namespace + suffixPassword)
	cfg.TLS.Enabled = v.GetBool(cfg.namespace + suffixTLS)
	cfg.TLS.CertPath = v.GetString(cfg.namespace + suffixCert)
	cfg.TLS.KeyPath = v.GetString(cfg.namespace + suffixKey)
	cfg.TLS.CaPath = v.GetString(cfg.namespace + suffixCA)
	cfg.TLS.ServerName = v.GetString(cfg.namespace + suffixServerName)
	cfg.TLS.EnableHostVerification = v.GetBool(cfg.namespace + suffixVerifyHost)
}

// GetPrimary returns primary configuration.
func (opt *Options) GetPrimary() *config.Configuration {
	opt.primary.Servers = strings.Split(opt.primary.servers, ",")
	return &opt.primary.Configuration
}

// Get returns auxiliary named configuration.
func (opt *Options) Get(namespace string) *config.Configuration {
	nsCfg, ok := opt.others[namespace]
	if !ok {
		nsCfg = &namespaceConfig{}
		opt.others[namespace] = nsCfg
	}
	nsCfg.Configuration.ApplyDefaults(&opt.primary.Configuration)
	if nsCfg.servers == "" {
		nsCfg.servers = opt.primary.servers
	}
	nsCfg.Servers = strings.Split(nsCfg.servers, ",")
	return &nsCfg.Configuration
}
