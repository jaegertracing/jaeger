// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cassandra

import (
	"flag"
	"strings"

	"github.com/spf13/viper"

	"github.com/uber/jaeger/pkg/cassandra/config"
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
	options := &Options{
		primary: &namespaceConfig{
			Configuration: config.Configuration{
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
