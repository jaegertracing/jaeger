package config

import (
  "time"

  "github.com/pkg/errors"
  "go.uber.org/zap"
  jaegerClient "github.com/uber/jaeger-client-go"
  jaegerClientLogger "github.com/uber/jaeger-client-go/log"
)

// Configuration describes the configuration properties needed to connect to an ElasticSearch cluster
type Configuration struct {
  ProxyHostPort              string
  ProxyIf                    string
  ProxyQueueSize             int
  ProxyBufferFlushIntervalMs int

}

// ClientBuilder creates new es.Client
type ClientBuilder interface {
  NewClient(logger *zap.Logger) (jaegerClient.Reporter, error)
  GetProxyHostPort() string
  GetProxyIf() string
  GetProxyQueueSize() int
  GetProxyBufferFlushIntervalMs() time.Duration
}

// NewClient creates a new ElasticSearch client
func (c *Configuration) NewClient(logger *zap.Logger) (jaegerClient.Reporter, error) {

  if c.ProxyHostPort == "" {
    return nil, errors.New("Host port empty")
  }

  transport, err := jaegerClient.NewUDPTransport(c.GetProxyHostPort(), 0)
  if err != nil {
    return nil, err
  }

  reporter := jaegerClient.NewRemoteReporter(
    transport,
    jaegerClient.ReporterOptions.QueueSize(c.GetProxyQueueSize()),
    jaegerClient.ReporterOptions.BufferFlushInterval(c.GetProxyBufferFlushIntervalMs()),
    jaegerClient.ReporterOptions.Logger(jaegerClientLogger.StdLogger),
    jaegerClient.ReporterOptions.Metrics(nil))

  return reporter, nil
}

// ApplyDefaults copies settings from source unless its own value is non-zero.
func (c *Configuration) ApplyDefaults(source *Configuration) {
  if c.ProxyHostPort == "" {
    c.ProxyHostPort = source.ProxyHostPort
  }
  if c.ProxyIf == "" {
    c.ProxyIf = source.ProxyIf
  }
  if c.ProxyQueueSize == 0 {
    c.ProxyQueueSize = source.ProxyQueueSize
  }
  if c.ProxyBufferFlushIntervalMs == 0 {
    c.ProxyBufferFlushIntervalMs = source.ProxyBufferFlushIntervalMs
  }
}

func (c *Configuration) GetProxyHostPort() string {
  return c.ProxyHostPort
}

func (c *Configuration) GetProxyIf() string {
  return c.ProxyIf
}

func (c *Configuration) GetProxyQueueSize() int {
  return c.ProxyQueueSize
}

func (c *Configuration) GetProxyBufferFlushIntervalMs() time.Duration {
  return time.Duration(c.ProxyBufferFlushIntervalMs) * time.Millisecond
}