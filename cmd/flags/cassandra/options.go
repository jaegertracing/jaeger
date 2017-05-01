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

	"github.com/uber/jaeger/pkg/cassandra/config"
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
	servers string
}

// NewOptions creates a new Options struct.
func NewOptions() *Options {
	return &Options{
		primary: &namespaceConfig{
			Configuration: config.Configuration{
				MaxRetryAttempts:   3,
				Keyspace:           "jaeger_v1_local",
				ProtoVersion:       4,
				ConnectionsPerHost: 2,
			},
			servers: "127.0.0.1",
		},
		others: make(map[string]*namespaceConfig),
	}
}

// Bind defines flags in the FlagSet for the primary namespace and optional other namespaces.
func (opt *Options) Bind(flags *flag.FlagSet, primaryNamespace string, otherNamespaces ...string) {
	bind(flags, primaryNamespace, opt.primary, &opt.primary.Configuration)
	for _, namespace := range otherNamespaces {
		nsCfg, ok := opt.others[namespace]
		if !ok {
			nsCfg = &namespaceConfig{}
			opt.others[namespace] = nsCfg
		}
		bind(flags, namespace, nsCfg, nil)
	}
}

// GetPrimary returns primary configuration.
func (opt *Options) GetPrimary() *config.Configuration {
	opt.primary.Servers = strings.Split(opt.primary.servers, ",")
	return &opt.primary.Configuration
}

// Get returns auxilary named configuration.
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

// bind defines a set of flags prefixed with the namespace and binds them to cassandra
// config struct, and a separate servers string pointer (because config struct uses array).
func bind(
	flags *flag.FlagSet,
	namespace string,
	cfg *namespaceConfig,
	defaults *config.Configuration,
) {
	if defaults == nil {
		defaults = &config.Configuration{}
	}
	flags.IntVar(
		&cfg.ConnectionsPerHost,
		namespace+".connections-per-host",
		defaults.ConnectionsPerHost,
		"The number of Cassandra connections from a single backend instance")
	flags.IntVar(
		&cfg.MaxRetryAttempts,
		namespace+".max-retry-attempts",
		defaults.MaxRetryAttempts,
		"The number of attempts when reading from Cassandra")
	flags.DurationVar(
		&cfg.Timeout,
		namespace+".timeout",
		defaults.Timeout,
		"Timeout used for queries")
	flags.StringVar(
		&cfg.servers,
		namespace+".servers",
		cfg.servers,
		"The comma-separated list of Cassandra servers")
	flags.IntVar(
		&cfg.Port,
		namespace+".port",
		defaults.Port,
		"The port for cassandra")
	flags.StringVar(
		&cfg.Keyspace,
		namespace+".keyspace",
		defaults.Keyspace,
		"The Cassandra keyspace for Jaeger data")
	flags.IntVar(
		&cfg.ProtoVersion,
		namespace+".proto-version",
		defaults.ProtoVersion,
		"The Cassandra protocol version")
	flags.DurationVar(
		&cfg.SocketKeepAlive,
		namespace+".socket-keep-alive",
		defaults.SocketKeepAlive,
		"Cassandra's keepalive period to use, enabled if > 0")
}
