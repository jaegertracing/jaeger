// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/cassandra/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
)

const (
	// session settings
	suffixEnabled            = ".enabled"
	suffixConnPerHost        = ".connections-per-host"
	suffixMaxRetryAttempts   = ".max-retry-attempts"
	suffixTimeout            = ".timeout"
	suffixConnectTimeout     = ".connect-timeout"
	suffixReconnectInterval  = ".reconnect-interval"
	suffixServers            = ".servers"
	suffixPort               = ".port"
	suffixKeyspace           = ".keyspace"
	suffixDC                 = ".local-dc"
	suffixConsistency        = ".consistency"
	suffixDisableCompression = ".disable-compression"
	suffixProtoVer           = ".proto-version"
	suffixSocketKeepAlive    = ".socket-keep-alive"
	suffixUsername           = ".username"
	suffixPassword           = ".password"
	suffixAuth               = ".basic.allowed-authenticators"
	// common storage settings
	suffixSpanStoreWriteCacheTTL = ".span-store-write-cache-ttl"
	suffixIndexTagsBlacklist     = ".index.tag-blacklist"
	suffixIndexTagsWhitelist     = ".index.tag-whitelist"
	suffixIndexLogs              = ".index.logs"
	suffixIndexTags              = ".index.tags"
	suffixIndexProcessTags       = ".index.process-tags"
)

// Options contains various type of Cassandra configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	NamespaceConfig        `mapstructure:",squash"`
	SpanStoreWriteCacheTTL time.Duration `mapstructure:"span_store_write_cache_ttl"`
	Index                  IndexConfig   `mapstructure:"index"`
}

// IndexConfig configures indexing.
// By default all indexing is enabled.
type IndexConfig struct {
	Logs         bool   `mapstructure:"logs"`
	Tags         bool   `mapstructure:"tags"`
	ProcessTags  bool   `mapstructure:"process_tags"`
	TagBlackList string `mapstructure:"tag_blacklist"`
	TagWhiteList string `mapstructure:"tag_whitelist"`
}

// the Servers field in config.Configuration is a list, which we cannot represent with flags.
// This struct adds a plain string field that can be bound to flags and is then parsed when
// preparing the actual config.Configuration.
type NamespaceConfig struct {
	config.Configuration `mapstructure:",squash"`
	namespace            string
	Enabled              bool `mapstructure:"-"`
}

// NewOptions creates a new Options struct.
func NewOptions(namespace string) *Options {
	// TODO all default values should be defined via cobra flags
	options := &Options{
		NamespaceConfig: NamespaceConfig{
			Configuration: config.DefaultConfiguration(),
			namespace:     namespace,
			Enabled:       true,
		},
		SpanStoreWriteCacheTTL: time.Hour * 12,
	}

	return options
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, opt.NamespaceConfig)
	flagSet.Duration(opt.namespace+suffixSpanStoreWriteCacheTTL,
		opt.SpanStoreWriteCacheTTL,
		"The duration to wait before rewriting an existing service or operation name")
	flagSet.String(
		opt.namespace+suffixIndexTagsBlacklist,
		opt.Index.TagBlackList,
		"The comma-separated list of span tags to blacklist from being indexed. All other tags will be indexed. Mutually exclusive with the whitelist option.")
	flagSet.String(
		opt.namespace+suffixIndexTagsWhitelist,
		opt.Index.TagWhiteList,
		"The comma-separated list of span tags to whitelist for being indexed. All other tags will not be indexed. Mutually exclusive with the blacklist option.")
	flagSet.Bool(
		opt.namespace+suffixIndexLogs,
		!opt.Index.Logs,
		"Controls log field indexing. Set to false to disable.")
	flagSet.Bool(
		opt.namespace+suffixIndexTags,
		!opt.Index.Tags,
		"Controls tag indexing. Set to false to disable.")
	flagSet.Bool(
		opt.namespace+suffixIndexProcessTags,
		!opt.Index.ProcessTags,
		"Controls process tag indexing. Set to false to disable.")
}

func addFlags(flagSet *flag.FlagSet, nsConfig NamespaceConfig) {
	tlsFlagsConfig := tlsFlagsConfig(nsConfig.namespace)
	tlsFlagsConfig.AddFlags(flagSet)

	if nsConfig.namespace != primaryStorageNamespace {
		flagSet.Bool(
			nsConfig.namespace+suffixEnabled,
			false,
			"Enable extra storage")
	}
	flagSet.Int(
		nsConfig.namespace+suffixConnPerHost,
		nsConfig.Connection.ConnectionsPerHost,
		"The number of Cassandra connections from a single backend instance")
	flagSet.Int(
		nsConfig.namespace+suffixMaxRetryAttempts,
		nsConfig.Query.MaxRetryAttempts,
		"The number of attempts when reading from Cassandra")
	flagSet.Duration(
		nsConfig.namespace+suffixTimeout,
		nsConfig.Query.Timeout,
		"Timeout used for queries. A Timeout of zero means no timeout")
	flagSet.Duration(
		nsConfig.namespace+suffixConnectTimeout,
		nsConfig.Connection.Timeout,
		"Timeout used for connections to Cassandra Servers")
	flagSet.Duration(
		nsConfig.namespace+suffixReconnectInterval,
		nsConfig.Connection.ReconnectInterval,
		"Reconnect interval to retry connecting to downed hosts")
	flagSet.String(
		nsConfig.namespace+suffixServers,
		strings.Join(nsConfig.Connection.Servers, ","),
		"The comma-separated list of Cassandra servers")
	flagSet.Int(
		nsConfig.namespace+suffixPort,
		nsConfig.Connection.Port,
		"The port for cassandra")
	flagSet.String(
		nsConfig.namespace+suffixKeyspace,
		nsConfig.Schema.Keyspace,
		"The Cassandra keyspace for Jaeger data")
	flagSet.String(
		nsConfig.namespace+suffixDC,
		nsConfig.Connection.LocalDC,
		"The name of the Cassandra local data center for DC Aware host selection")
	flagSet.String(
		nsConfig.namespace+suffixConsistency,
		nsConfig.Query.Consistency,
		"The Cassandra consistency level, e.g. ANY, ONE, TWO, THREE, QUORUM, ALL, LOCAL_QUORUM, EACH_QUORUM, LOCAL_ONE (default LOCAL_ONE)")
	flagSet.Bool(
		nsConfig.namespace+suffixDisableCompression,
		false,
		"Disables the use of the default Snappy Compression while connecting to the Cassandra Cluster if set to true. This is useful for connecting to Cassandra Clusters(like Azure Cosmos Db with Cassandra API) that do not support SnappyCompression")
	flagSet.Int(
		nsConfig.namespace+suffixProtoVer,
		nsConfig.Connection.ProtoVersion,
		"The Cassandra protocol version")
	flagSet.Duration(
		nsConfig.namespace+suffixSocketKeepAlive,
		nsConfig.Connection.SocketKeepAlive,
		"Cassandra's keepalive period to use, enabled if > 0")
	flagSet.String(
		nsConfig.namespace+suffixUsername,
		nsConfig.Connection.Authenticator.Basic.Username,
		"Username for password authentication for Cassandra")
	flagSet.String(
		nsConfig.namespace+suffixPassword,
		nsConfig.Connection.Authenticator.Basic.Password,
		"Password for password authentication for Cassandra")
	flagSet.String(
		nsConfig.namespace+suffixAuth,
		"",
		"The comma-separated list of allowed password authenticators for Cassandra."+
			"If none are specified, there is a default 'approved' list that is used "+
			"(https://github.com/gocql/gocql/blob/34fdeebefcbf183ed7f916f931aa0586fdaa1b40/conn.go#L27). "+
			"If a non-empty list is provided, only specified authenticators are allowed.")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.NamespaceConfig.initFromViper(v)
	opt.SpanStoreWriteCacheTTL = v.GetDuration(opt.NamespaceConfig.namespace + suffixSpanStoreWriteCacheTTL)
	opt.Index.TagBlackList = stripWhiteSpace(v.GetString(opt.NamespaceConfig.namespace + suffixIndexTagsBlacklist))
	opt.Index.TagWhiteList = stripWhiteSpace(v.GetString(opt.NamespaceConfig.namespace + suffixIndexTagsWhitelist))
	opt.Index.Tags = v.GetBool(opt.NamespaceConfig.namespace + suffixIndexTags)
	opt.Index.Logs = v.GetBool(opt.NamespaceConfig.namespace + suffixIndexLogs)
	opt.Index.ProcessTags = v.GetBool(opt.NamespaceConfig.namespace + suffixIndexProcessTags)
}

func tlsFlagsConfig(namespace string) tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: namespace,
	}
}

func (cfg *NamespaceConfig) initFromViper(v *viper.Viper) {
	tlsFlagsConfig := tlsFlagsConfig(cfg.namespace)
	if cfg.namespace != primaryStorageNamespace {
		cfg.Enabled = v.GetBool(cfg.namespace + suffixEnabled)
	}
	cfg.Connection.ConnectionsPerHost = v.GetInt(cfg.namespace + suffixConnPerHost)
	cfg.Query.MaxRetryAttempts = v.GetInt(cfg.namespace + suffixMaxRetryAttempts)
	cfg.Query.Timeout = v.GetDuration(cfg.namespace + suffixTimeout)
	cfg.Connection.Timeout = v.GetDuration(cfg.namespace + suffixConnectTimeout)
	cfg.Connection.ReconnectInterval = v.GetDuration(cfg.namespace + suffixReconnectInterval)
	servers := stripWhiteSpace(v.GetString(cfg.namespace + suffixServers))
	cfg.Connection.Servers = strings.Split(servers, ",")
	cfg.Connection.Port = v.GetInt(cfg.namespace + suffixPort)
	cfg.Schema.Keyspace = v.GetString(cfg.namespace + suffixKeyspace)
	cfg.Connection.LocalDC = v.GetString(cfg.namespace + suffixDC)
	cfg.Query.Consistency = v.GetString(cfg.namespace + suffixConsistency)
	cfg.Connection.ProtoVersion = v.GetInt(cfg.namespace + suffixProtoVer)
	cfg.Connection.SocketKeepAlive = v.GetDuration(cfg.namespace + suffixSocketKeepAlive)
	cfg.Connection.Authenticator.Basic.Username = v.GetString(cfg.namespace + suffixUsername)
	cfg.Connection.Authenticator.Basic.Password = v.GetString(cfg.namespace + suffixPassword)
	authentication := stripWhiteSpace(v.GetString(cfg.namespace + suffixAuth))
	cfg.Connection.Authenticator.Basic.AllowedAuthenticators = strings.Split(authentication, ",")
	cfg.Schema.DisableCompression = v.GetBool(cfg.namespace + suffixDisableCompression)
	var err error
	tlsCfg, err := tlsFlagsConfig.InitFromViper(v)
	if err != nil {
		// TODO refactor to be able to return error
		log.Fatal(err)
	}
	cfg.Connection.TLS = tlsCfg
}

func (opt *Options) GetConfig() config.Configuration {
	return opt.NamespaceConfig.Configuration
}

// TagIndexBlacklist returns the list of blacklisted tags
func (opt *Options) TagIndexBlacklist() []string {
	if len(opt.Index.TagBlackList) > 0 {
		return strings.Split(opt.Index.TagBlackList, ",")
	}

	return nil
}

// TagIndexWhitelist returns the list of whitelisted tags
func (opt *Options) TagIndexWhitelist() []string {
	if len(opt.Index.TagWhiteList) > 0 {
		return strings.Split(opt.Index.TagWhiteList, ",")
	}

	return nil
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.ReplaceAll(str, " ", "")
}
