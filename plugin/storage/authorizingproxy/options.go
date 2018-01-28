package authorizingproxy

import (
  "flag"

  "github.com/spf13/viper"
  
  "github.com/jaegertracing/jaeger/plugin/storage/authorizingproxy/config"
)

const (
  suffixProxyHostPort              = ".proxy-host-port"
  suffixProxyIf                    = ".proxy-if"
  suffixProxyBatchSize             = ".proxy-batch-size"
  suffixProxyBatchFlushIntervalMs  = ".proxy-batch-flush-interval-ms"
)

type Options struct {
  primary *namespaceConfig
  others map[string]*namespaceConfig
}

// the Servers field in config.Configuration is a list, which we cannot represent with flags.
// This struct adds a plain string field that can be bound to flags and is then parsed when
// preparing the actual config.Configuration.
type namespaceConfig struct {
  config.Configuration
  namespace string
}

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string, otherNamespaces ...string) *Options {
  // TODO all default values should be defined via cobra flags
  options := &Options{
    primary: &namespaceConfig{
      Configuration: config.Configuration{
        ProxyHostPort:              "",
        ProxyIf:                    "",
        ProxyBatchSize:             50,
        ProxyBatchFlushIntervalMs:  500,
      },
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
  flagSet.String(
    nsConfig.namespace+suffixProxyHostPort,
    nsConfig.ProxyHostPort,
    "The host port string of the collector to proxy the requests to. Can be comma delimited list.")
  flagSet.String(
    nsConfig.namespace+suffixProxyIf,
    nsConfig.ProxyIf,
    "The condition under which the requests should be proxied.")
  flagSet.Int(
    nsConfig.namespace+suffixProxyBatchSize,
    nsConfig.ProxyBatchSize,
    "Batch size - maximum number of items to send to a collector in a single batch.")
  flagSet.Int(
    nsConfig.namespace+suffixProxyBatchFlushIntervalMs,
    nsConfig.ProxyBatchFlushIntervalMs,
    "Batch flush interval - maximum number of milliseconds to wait until flushing a batch, even if batch size hasn't been reached.")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
  initFromViper(opt.primary, v)
  for _, cfg := range opt.others {
    initFromViper(cfg, v)
  }
}

func initFromViper(cfg *namespaceConfig, v *viper.Viper) {
  cfg.ProxyHostPort = v.GetString(cfg.namespace + suffixProxyHostPort)
  cfg.ProxyIf = v.GetString(cfg.namespace + suffixProxyIf)
  cfg.ProxyBatchSize = v.GetInt(cfg.namespace + suffixProxyBatchSize)
  cfg.ProxyBatchFlushIntervalMs = v.GetInt(cfg.namespace + suffixProxyBatchFlushIntervalMs)
}

// GetPrimary returns primary configuration.
func (opt *Options) GetPrimary() *config.Configuration {
  return &opt.primary.Configuration
}