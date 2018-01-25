package authorizingproxy

import (
  "flag"
  "github.com/spf13/viper"
  "github.com/jaegertracing/jaeger/plugin/storage/authorizingproxy/config"
)

const (
  suffixProxyHostPort              = ".proxy-hostport"
  suffixProxyIf                    = ".proxy-if"
  suffixProxyQueueSize             = ".proxy-queue-size"
  suffixProxyBufferFlushIntervalMs = ".proxy-buffer-flush-interval-ms"
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
        ProxyQueueSize:             0,
        ProxyBufferFlushIntervalMs: 0,
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
    "The host port string of the collector to proxy the requests to")
  flagSet.String(
    nsConfig.namespace+suffixProxyIf,
    nsConfig.ProxyIf,
    "The condition under which the requests should be proxied")
  flagSet.Int(
    nsConfig.namespace+suffixProxyQueueSize,
    nsConfig.ProxyQueueSize,
    "queue size - TODO figure out")
  flagSet.Int(
    nsConfig.namespace+suffixProxyBufferFlushIntervalMs,
    nsConfig.ProxyBufferFlushIntervalMs,
    "buffer flush interval - TODO figure out")
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
  cfg.ProxyQueueSize = v.GetInt(cfg.namespace + suffixProxyQueueSize)
  cfg.ProxyBufferFlushIntervalMs = v.GetInt(cfg.namespace + suffixProxyBufferFlushIntervalMs)
}

// GetPrimary returns primary configuration.
func (opt *Options) GetPrimary() *config.Configuration {
  return &opt.primary.Configuration
}