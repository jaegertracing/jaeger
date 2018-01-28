package config

import (
  "strings"
  "time"

  "github.com/pkg/errors"
  "github.com/uber/jaeger-lib/metrics"
  "go.uber.org/zap"

  agentReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
  "github.com/jaegertracing/jaeger/pkg/discovery"
)

// Configuration describes the configuration properties needed to connect to an ElasticSearch cluster
type Configuration struct {
  ProxyHostPort              string
  ProxyIf                    string
  ProxyBatchSize             int
  ProxyBatchFlushIntervalMs  int

}

// ClientBuilder creates new es.Client
type ClientBuilder interface {
  NewClient(metricsFactory metrics.Factory, logger *zap.Logger) (*agentReporter.Reporter, error)
  GetProxyHostPort() string
  GetProxyIf() string
  GetProxyBatchSize() int
  GetProxyBatchFlushIntervalMs() time.Duration
}

// NewClient creates a new ElasticSearch client
func (c *Configuration) NewClient(metricsFactory metrics.Factory, logger *zap.Logger) (*agentReporter.Reporter, error) {

  if c.ProxyHostPort == "" {
    return nil, errors.New("Host port empty")
  }

  hostPorts := strings.Split(c.GetProxyHostPort(), ",")
  discoverer := discovery.FixedDiscoverer(hostPorts)
  notifier := &discovery.Dispatcher{}

  builder := agentReporter.NewBuilder()
  builder.WithDiscoverer(discoverer)
  builder.WithDiscoveryNotifier(notifier)
  return builder.CreateReporter(metricsFactory, logger)
}

// ApplyDefaults copies settings from source unless its own value is non-zero.
func (c *Configuration) ApplyDefaults(source *Configuration) {
  if c.ProxyHostPort == "" {
    c.ProxyHostPort = source.ProxyHostPort
  }
  if c.ProxyIf == "" {
    c.ProxyIf = source.ProxyIf
  }
  if c.ProxyBatchSize == 0 {
    c.ProxyBatchSize = source.ProxyBatchSize
  }
  if c.ProxyBatchFlushIntervalMs == 0 {
    c.ProxyBatchFlushIntervalMs = source.ProxyBatchFlushIntervalMs
  }
}

func (c *Configuration) GetProxyHostPort() string {
  return c.ProxyHostPort
}

func (c *Configuration) GetProxyIf() string {
  return c.ProxyIf
}

func (c *Configuration) GetProxyBatchSize() int {
  return c.ProxyBatchSize
}

func (c *Configuration) GetProxyBatchFlushIntervalMs() time.Duration {
  return time.Duration(c.ProxyBatchFlushIntervalMs) * time.Millisecond
}