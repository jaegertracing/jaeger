// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/pkg/config/corscfg"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	flagDynQueueSizeMemory     = "collector.queue-size-memory"
	flagNumWorkers             = "collector.num-workers"
	flagQueueSize              = "collector.queue-size"
	flagCollectorTags          = "collector.tags"
	flagSpanSizeMetricsEnabled = "collector.enable-span-size-metrics"

	flagSuffixHostPort = "host-port"

	flagSuffixHTTPReadTimeout       = "read-timeout"
	flagSuffixHTTPReadHeaderTimeout = "read-header-timeout"
	flagSuffixHTTPIdleTimeout       = "idle-timeout"

	flagSuffixGRPCMaxReceiveMessageLength = "max-message-size"
	flagSuffixGRPCMaxConnectionAge        = "max-connection-age"
	flagSuffixGRPCMaxConnectionAgeGrace   = "max-connection-age-grace"

	flagCollectorOTLPEnabled = "collector.otlp.enabled"

	flagZipkinHTTPHostPort     = "collector.zipkin.host-port"
	flagZipkinKeepAliveEnabled = "collector.zipkin.keep-alive"

	// DefaultNumWorkers is the default number of workers consuming from the processor queue
	DefaultNumWorkers = 50
	// DefaultQueueSize is the size of the processor's queue
	DefaultQueueSize = 2000
	// DefaultGRPCMaxReceiveMessageLength is the default max receivable message size for the gRPC Collector
	DefaultGRPCMaxReceiveMessageLength = 4 * 1024 * 1024
)

var grpcServerFlagsCfg = serverFlagsConfig{
	// for legacy reasons the prefixes are different
	prefix: "collector.grpc-server",
	tls: tlscfg.ServerFlagsConfig{
		Prefix: "collector.grpc",
	},
}

var httpServerFlagsCfg = serverFlagsConfig{
	// for legacy reasons the prefixes are different
	prefix: "collector.http-server",
	tls: tlscfg.ServerFlagsConfig{
		Prefix: "collector.http",
	},
	corsCfg: corscfg.Flags{
		Prefix: "collector.otlp.http",
	},
}

var otlpServerFlagsCfg = struct {
	GRPC serverFlagsConfig
	HTTP serverFlagsConfig
}{
	GRPC: serverFlagsConfig{
		prefix: "collector.otlp.grpc",
		tls: tlscfg.ServerFlagsConfig{
			Prefix: "collector.otlp.grpc",
		},
	},
	HTTP: serverFlagsConfig{
		prefix: "collector.otlp.http",
		tls: tlscfg.ServerFlagsConfig{
			Prefix: "collector.otlp.http",
		},
		corsCfg: corscfg.Flags{
			Prefix: "collector.otlp.http",
		},
	},
}

var zipkinServerFlagsCfg = serverFlagsConfig{
	prefix: "collector.zipkin",
	tls: tlscfg.ServerFlagsConfig{
		Prefix: "collector.zipkin",
	},
	corsCfg: corscfg.Flags{
		Prefix: "collector.zipkin",
	},
}

// CollectorOptions holds configuration for collector
type CollectorOptions struct {
	// DynQueueSizeMemory determines how much memory to use for the queue
	DynQueueSizeMemory uint
	// QueueSize is the size of collector's queue
	QueueSize uint
	// NumWorkers is the number of internal workers in a collector
	NumWorkers int
	// HTTP section defines options for HTTP server
	HTTP confighttp.ServerConfig
	// GRPC section defines options for gRPC server
	GRPC configgrpc.ServerConfig
	// OTLP section defines options for servers accepting OpenTelemetry OTLP format
	OTLP struct {
		Enabled bool
		GRPC    configgrpc.ServerConfig
		HTTP    confighttp.ServerConfig
	}
	// Zipkin section defines options for Zipkin HTTP server
	Zipkin struct {
		confighttp.ServerConfig
		KeepAlive bool
	}
	// CollectorTags is the string representing collector tags to append to each and every span
	CollectorTags map[string]string
	// SpanSizeMetricsEnabled determines whether to enable metrics based on processed span size
	SpanSizeMetricsEnabled bool

	Tenancy tenancy.Options
}

type serverFlagsConfig struct {
	prefix  string
	tls     tlscfg.ServerFlagsConfig
	corsCfg corscfg.Flags
}

// AddFlags adds flags for CollectorOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(flagNumWorkers, DefaultNumWorkers, "The number of workers pulling items from the queue")
	flagSet.Int(flagQueueSize, DefaultQueueSize, "The queue size of the collector")
	flagSet.Uint(flagDynQueueSizeMemory, 0, "(experimental) The max memory size in MiB to use for the dynamic queue.")
	flagSet.String(flagCollectorTags, "", "One or more tags to be added to the Process tags of all spans passing through this collector. Ex: key1=value1,key2=${envVar:defaultValue}")
	flagSet.Bool(flagSpanSizeMetricsEnabled, false, "Enables metrics based on processed span size, which are more expensive to calculate.")

	addHTTPFlags(flagSet, httpServerFlagsCfg, ports.PortToHostPort(ports.CollectorHTTP))
	addGRPCFlags(flagSet, grpcServerFlagsCfg, ports.PortToHostPort(ports.CollectorGRPC))

	flagSet.Bool(flagCollectorOTLPEnabled, true, "Enables OpenTelemetry OTLP receiver on dedicated HTTP and gRPC ports")
	addHTTPFlags(flagSet, otlpServerFlagsCfg.HTTP, ":4318")
	otlpServerFlagsCfg.HTTP.corsCfg.AddFlags(flagSet)
	addGRPCFlags(flagSet, otlpServerFlagsCfg.GRPC, ":4317")

	flagSet.String(flagZipkinHTTPHostPort, "", "The host:port (e.g. 127.0.0.1:9411 or :9411) of the collector's Zipkin server (disabled by default)")
	flagSet.Bool(flagZipkinKeepAliveEnabled, true, "KeepAlive configures allow Keep-Alive for Zipkin HTTP server (enabled by default)")
	zipkinServerFlagsCfg.tls.AddFlags(flagSet)
	zipkinServerFlagsCfg.corsCfg.AddFlags(flagSet)

	tenancy.AddFlags(flagSet)
}

func addHTTPFlags(flagSet *flag.FlagSet, cfg serverFlagsConfig, defaultHostPort string) {
	flagSet.String(cfg.prefix+"."+flagSuffixHostPort, defaultHostPort, "The host:port (e.g. 127.0.0.1:12345 or :12345) of the collector's HTTP server")
	flagSet.Duration(cfg.prefix+"."+flagSuffixHTTPIdleTimeout, 0, "See https://pkg.go.dev/net/http#Server")
	flagSet.Duration(cfg.prefix+"."+flagSuffixHTTPReadTimeout, 0, "See https://pkg.go.dev/net/http#Server")
	flagSet.Duration(cfg.prefix+"."+flagSuffixHTTPReadHeaderTimeout, 2*time.Second, "See https://pkg.go.dev/net/http#Server")
	cfg.tls.AddFlags(flagSet)
}

func addGRPCFlags(flagSet *flag.FlagSet, cfg serverFlagsConfig, defaultHostPort string) {
	flagSet.String(
		cfg.prefix+"."+flagSuffixHostPort,
		defaultHostPort,
		"The host:port (e.g. 127.0.0.1:12345 or :12345) of the collector's gRPC server")
	flagSet.Int(
		cfg.prefix+"."+flagSuffixGRPCMaxReceiveMessageLength,
		DefaultGRPCMaxReceiveMessageLength,
		"The maximum receivable message size for the collector's gRPC server")
	flagSet.Duration(
		cfg.prefix+"."+flagSuffixGRPCMaxConnectionAge,
		0,
		"The maximum amount of time a connection may exist. Set this value to a few seconds or minutes on highly elastic environments, so that clients discover new collector nodes frequently. See https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters")
	flagSet.Duration(
		cfg.prefix+"."+flagSuffixGRPCMaxConnectionAgeGrace,
		0,
		"The additive period after MaxConnectionAge after which the connection will be forcibly closed. See https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters")
	cfg.tls.AddFlags(flagSet)
}

func initHTTPFromViper(v *viper.Viper, opts *confighttp.ServerConfig, cfg serverFlagsConfig) error {
	tlsHTTPCfg, err := cfg.tls.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse HTTP TLS options: %w", err)
	}
	opts.TLSSetting = tlsHTTPCfg
	opts.Endpoint = ports.FormatHostPort(v.GetString(cfg.prefix + "." + flagSuffixHostPort))
	opts.IdleTimeout = v.GetDuration(cfg.prefix + "." + flagSuffixHTTPIdleTimeout)
	opts.ReadTimeout = v.GetDuration(cfg.prefix + "." + flagSuffixHTTPReadTimeout)
	opts.ReadHeaderTimeout = v.GetDuration(cfg.prefix + "." + flagSuffixHTTPReadHeaderTimeout)
	opts.CORS = cfg.corsCfg.InitFromViper(v)

	return nil
}

func initGRPCFromViper(v *viper.Viper, opts *configgrpc.ServerConfig, cfg serverFlagsConfig) error {
	tlsGRPCCfg, err := cfg.tls.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse GRPC TLS options: %w", err)
	}
	opts.TLSSetting = tlsGRPCCfg
	opts.NetAddr.Endpoint = ports.FormatHostPort(v.GetString(cfg.prefix + "." + flagSuffixHostPort))
	opts.MaxRecvMsgSizeMiB = v.GetInt(cfg.prefix+"."+flagSuffixGRPCMaxReceiveMessageLength) / (1024 * 1024)
	opts.Keepalive = &configgrpc.KeepaliveServerConfig{
		ServerParameters: &configgrpc.KeepaliveServerParameters{
			MaxConnectionAge:      v.GetDuration(cfg.prefix + "." + flagSuffixGRPCMaxConnectionAge),
			MaxConnectionAgeGrace: v.GetDuration(cfg.prefix + "." + flagSuffixGRPCMaxConnectionAgeGrace),
		},
	}

	return nil
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *CollectorOptions) InitFromViper(v *viper.Viper, _ *zap.Logger) (*CollectorOptions, error) {
	cOpts.CollectorTags = flags.ParseJaegerTags(v.GetString(flagCollectorTags))
	cOpts.NumWorkers = v.GetInt(flagNumWorkers)
	cOpts.QueueSize = v.GetUint(flagQueueSize)
	cOpts.DynQueueSizeMemory = v.GetUint(flagDynQueueSizeMemory) * 1024 * 1024 // we receive in MiB and store in bytes
	cOpts.SpanSizeMetricsEnabled = v.GetBool(flagSpanSizeMetricsEnabled)
	cOpts.Tenancy = tenancy.InitFromViper(v)

	if err := initHTTPFromViper(v, &cOpts.HTTP, httpServerFlagsCfg); err != nil {
		return cOpts, fmt.Errorf("failed to parse HTTP server options: %w", err)
	}

	if err := initGRPCFromViper(v, &cOpts.GRPC, grpcServerFlagsCfg); err != nil {
		return cOpts, fmt.Errorf("failed to parse gRPC server options: %w", err)
	}

	cOpts.OTLP.Enabled = v.GetBool(flagCollectorOTLPEnabled)

	if err := initHTTPFromViper(v, &cOpts.OTLP.HTTP, otlpServerFlagsCfg.HTTP); err != nil {
		return cOpts, fmt.Errorf("failed to parse OTLP/HTTP server options: %w", err)
	}
	if err := initGRPCFromViper(v, &cOpts.OTLP.GRPC, otlpServerFlagsCfg.GRPC); err != nil {
		return cOpts, fmt.Errorf("failed to parse OTLP/gRPC server options: %w", err)
	}

	cOpts.Zipkin.KeepAlive = v.GetBool(flagZipkinKeepAliveEnabled)

	if err := initHTTPFromViper(v, &cOpts.Zipkin.ServerConfig, zipkinServerFlagsCfg); err != nil {
		return cOpts, fmt.Errorf("failed to parse Zipkin server options: %w", err)
	}

	return cOpts, nil
}
