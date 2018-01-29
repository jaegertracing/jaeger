package authorizingproxy

import (
  "flag"

  "github.com/spf13/viper"
  "github.com/uber/jaeger-lib/metrics"
  "go.uber.org/zap"

  agentReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
  "github.com/jaegertracing/jaeger/plugin/storage/authorizingproxy/config"
  proxyDepStore "github.com/jaegertracing/jaeger/plugin/storage/authorizingproxy/dependencystore"
  proxySpanStore "github.com/jaegertracing/jaeger/plugin/storage/authorizingproxy/spanstore"
  "github.com/jaegertracing/jaeger/storage/dependencystore"
  "github.com/jaegertracing/jaeger/storage/spanstore"
)

type Factory struct {
  Options *Options

  metricsFactory metrics.Factory
  logger         *zap.Logger

  primaryConfig config.ClientBuilder
  primaryClient *agentReporter.Reporter
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
  return &Factory{
    Options: NewOptions("authorizingproxy"),
  }
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
  f.Options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper) {
  f.Options.InitFromViper(v)
  f.primaryConfig = f.Options.GetPrimary()
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
  f.metricsFactory, f.logger = metricsFactory, logger
  f.primaryConfig = f.Options.GetPrimary()

  primaryClient, err := f.primaryConfig.NewClient(metricsFactory, logger)
  if err != nil {
    return err
  }
  f.primaryClient = primaryClient
  // TODO init archive (cf. https://github.com/jaegertracing/jaeger/pull/604)
  return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
  return proxySpanStore.NewSpanReader(f.primaryClient, f.logger, f.metricsFactory), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
  cfg := f.primaryConfig
  return proxySpanStore.NewSpanWriter(f.primaryClient,
    f.logger,
    f.metricsFactory,
    cfg.GetProxyBatchSize(),
    cfg.GetProxyBatchFlushIntervalMs(),
    cfg.GetProxyIf()), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
  return proxyDepStore.NewDependencyStore(f.primaryClient, f.logger), nil
}