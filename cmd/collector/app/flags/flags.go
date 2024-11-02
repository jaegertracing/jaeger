// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"
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
}

var otlpServerFlagsCfg = struct {
	GRPC serverFlagsConfig
	HTTP serverFlagsConfig
}{
	GRPC: serverFlagsConfig{
		prefix: "collector.otlp.grpc",
		tls: tlscfg.ServerFlagsConfig{
			Prefix:                   "collector.otlp.grpc",
			EnableCertReloadInterval: true,
		},
	},
	HTTP: serverFlagsConfig{
		prefix: "collector.otlp.http",
		tls: tlscfg.ServerFlagsConfig{
			Prefix:                   "collector.otlp.http",
			EnableCertReloadInterval: true,
		},
	},
}

var tlsZipkinFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "collector.zipkin",
}

var corsZipkinFlags = corscfg.Flags{
	Prefix: "collector.zipkin",
}

var corsOTLPFlags = corscfg.Flags{
	Prefix: "collector.otlp.http",
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
	HTTP HTTPOptions
	// GRPC section defines options for gRPC server
	GRPC GRPCOptions
	// OTLP section defines options for servers accepting OpenTelemetry OTLP format
	OTLP struct {
		Enabled bool
		GRPC    GRPCOptions
		HTTP    HTTPOptions
	}
	// Zipkin section defines options for Zipkin HTTP server
	Zipkin struct {
		// HTTPHostPort is the host:port address that the Zipkin collector service listens in on for http requests
		HTTPHostPort string
		// TLS configures secure transport for Zipkin endpoint to collect spans
		TLS tlscfg.Options
		// CORS allows CORS requests , sets the values for Allowed Headers and Allowed Origins.
		CORS corscfg.Options
		// KeepAlive configures allow Keep-Alive for Zipkin HTTP server
		KeepAlive bool
	}
	// CollectorTags is the string representing collector tags to append to each and every span
	CollectorTags map[string]string
	// SpanSizeMetricsEnabled determines whether to enable metrics based on processed span size
	SpanSizeMetricsEnabled bool
}

type serverFlagsConfig struct {
	prefix string
	tls    tlscfg.ServerFlagsConfig
}

// HTTPOptions defines options for an HTTP server
type HTTPOptions struct {
	// HostPort is the host:port address that the server listens on
	HostPort string
	// TLS configures secure transport for HTTP endpoint
	TLS tlscfg.Options
	// ReadTimeout sets the respective parameter of http.Server
	ReadTimeout time.Duration
	// ReadHeaderTimeout sets the respective parameter of http.Server
	ReadHeaderTimeout time.Duration
	// IdleTimeout sets the respective parameter of http.Server
	IdleTimeout time.Duration
	// CORS allows CORS requests , sets the values for Allowed Headers and Allowed Origins.
	CORS corscfg.Options
}

// GRPCOptions defines options for a gRPC server
type GRPCOptions struct {
	// HostPort is the host:port address that the collector service listens in on for gRPC requests
	HostPort string
	// TLS configures secure transport for gRPC endpoint to collect spans
	TLS tlscfg.Options
	// MaxReceiveMessageLength is the maximum message size receivable by the gRPC Collector.
	MaxReceiveMessageLength int
	// MaxConnectionAge is a duration for the maximum amount of time a connection may exist.
	// See gRPC's keepalive.ServerParameters#MaxConnectionAge.
	MaxConnectionAge time.Duration
	// MaxConnectionAgeGrace is an additive period after MaxConnectionAge after which the connection will be forcibly closed.
	// See gRPC's keepalive.ServerParameters#MaxConnectionAgeGrace.
	MaxConnectionAgeGrace time.Duration
	// Tenancy configures tenancy for endpoints that collect spans
	Tenancy tenancy.Options
}

// AddFlags adds flags for CollectorOptions
func AddFlags(flags *flag.FlagSet) {
	flags.Int(flagNumWorkers, DefaultNumWorkers, "The number of workers pulling items from the queue")
	flags.Int(flagQueueSize, DefaultQueueSize, "The queue size of the collector")
	flags.Uint(flagDynQueueSizeMemory, 0, "(experimental) The max memory size in MiB to use for the dynamic queue.")
	flags.String(flagCollectorTags, "", "One or more tags to be added to the Process tags of all spans passing through this collector. Ex: key1=value1,key2=${envVar:defaultValue}")
	flags.Bool(flagSpanSizeMetricsEnabled, false, "Enables metrics based on processed span size, which are more expensive to calculate.")

	addHTTPFlags(flags, httpServerFlagsCfg, ports.PortToHostPort(ports.CollectorHTTP))
	addGRPCFlags(flags, grpcServerFlagsCfg, ports.PortToHostPort(ports.CollectorGRPC))

	flags.Bool(flagCollectorOTLPEnabled, true, "Enables OpenTelemetry OTLP receiver on dedicated HTTP and gRPC ports")
	addHTTPFlags(flags, otlpServerFlagsCfg.HTTP, ":4318")
	corsOTLPFlags.AddFlags(flags)
	addGRPCFlags(flags, otlpServerFlagsCfg.GRPC, ":4317")

	flags.String(flagZipkinHTTPHostPort, "", "The host:port (e.g. 127.0.0.1:9411 or :9411) of the collector's Zipkin server (disabled by default)")
	flags.Bool(flagZipkinKeepAliveEnabled, true, "KeepAlive configures allow Keep-Alive for Zipkin HTTP server (enabled by default)")
	tlsZipkinFlagsConfig.AddFlags(flags)
	corsZipkinFlags.AddFlags(flags)

	tenancy.AddFlags(flags)
}

func addHTTPFlags(flags *flag.FlagSet, cfg serverFlagsConfig, defaultHostPort string) {
	flags.String(cfg.prefix+"."+flagSuffixHostPort, defaultHostPort, "The host:port (e.g. 127.0.0.1:12345 or :12345) of the collector's HTTP server")
	flags.Duration(cfg.prefix+"."+flagSuffixHTTPIdleTimeout, 0, "See https://pkg.go.dev/net/http#Server")
	flags.Duration(cfg.prefix+"."+flagSuffixHTTPReadTimeout, 0, "See https://pkg.go.dev/net/http#Server")
	flags.Duration(cfg.prefix+"."+flagSuffixHTTPReadHeaderTimeout, 2*time.Second, "See https://pkg.go.dev/net/http#Server")
	cfg.tls.AddFlags(flags)
}

func addGRPCFlags(flags *flag.FlagSet, cfg serverFlagsConfig, defaultHostPort string) {
	flags.String(
		cfg.prefix+"."+flagSuffixHostPort,
		defaultHostPort,
		"The host:port (e.g. 127.0.0.1:12345 or :12345) of the collector's gRPC server")
	flags.Int(
		cfg.prefix+"."+flagSuffixGRPCMaxReceiveMessageLength,
		DefaultGRPCMaxReceiveMessageLength,
		"The maximum receivable message size for the collector's gRPC server")
	flags.Duration(
		cfg.prefix+"."+flagSuffixGRPCMaxConnectionAge,
		0,
		"The maximum amount of time a connection may exist. Set this value to a few seconds or minutes on highly elastic environments, so that clients discover new collector nodes frequently. See https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters")
	flags.Duration(
		cfg.prefix+"."+flagSuffixGRPCMaxConnectionAgeGrace,
		0,
		"The additive period after MaxConnectionAge after which the connection will be forcibly closed. See https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters")
	cfg.tls.AddFlags(flags)
}

func (opts *HTTPOptions) initFromViper(v *viper.Viper, _ *zap.Logger, cfg serverFlagsConfig) error {
	opts.HostPort = ports.FormatHostPort(v.GetString(cfg.prefix + "." + flagSuffixHostPort))
	opts.IdleTimeout = v.GetDuration(cfg.prefix + "." + flagSuffixHTTPIdleTimeout)
	opts.ReadTimeout = v.GetDuration(cfg.prefix + "." + flagSuffixHTTPReadTimeout)
	opts.ReadHeaderTimeout = v.GetDuration(cfg.prefix + "." + flagSuffixHTTPReadHeaderTimeout)
	tlsOpts, err := cfg.tls.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse HTTP TLS options: %w", err)
	}
	opts.TLS = tlsOpts
	return nil
}

func (opts *GRPCOptions) initFromViper(v *viper.Viper, _ *zap.Logger, cfg serverFlagsConfig) error {
	opts.HostPort = ports.FormatHostPort(v.GetString(cfg.prefix + "." + flagSuffixHostPort))
	opts.MaxReceiveMessageLength = v.GetInt(cfg.prefix + "." + flagSuffixGRPCMaxReceiveMessageLength)
	opts.MaxConnectionAge = v.GetDuration(cfg.prefix + "." + flagSuffixGRPCMaxConnectionAge)
	opts.MaxConnectionAgeGrace = v.GetDuration(cfg.prefix + "." + flagSuffixGRPCMaxConnectionAgeGrace)
	tlsOpts, err := cfg.tls.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse gRPC TLS options: %w", err)
	}
	opts.TLS = tlsOpts
	opts.Tenancy = tenancy.InitFromViper(v)

	return nil
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *CollectorOptions) InitFromViper(v *viper.Viper, logger *zap.Logger) (*CollectorOptions, error) {
	cOpts.CollectorTags = flags.ParseJaegerTags(v.GetString(flagCollectorTags))
	cOpts.NumWorkers = v.GetInt(flagNumWorkers)
	cOpts.QueueSize = v.GetUint(flagQueueSize)
	cOpts.DynQueueSizeMemory = v.GetUint(flagDynQueueSizeMemory) * 1024 * 1024 // we receive in MiB and store in bytes
	cOpts.SpanSizeMetricsEnabled = v.GetBool(flagSpanSizeMetricsEnabled)

	if err := cOpts.HTTP.initFromViper(v, logger, httpServerFlagsCfg); err != nil {
		return cOpts, fmt.Errorf("failed to parse HTTP server options: %w", err)
	}

	if err := cOpts.GRPC.initFromViper(v, logger, grpcServerFlagsCfg); err != nil {
		return cOpts, fmt.Errorf("failed to parse gRPC server options: %w", err)
	}

	cOpts.OTLP.Enabled = v.GetBool(flagCollectorOTLPEnabled)
	if err := cOpts.OTLP.HTTP.initFromViper(v, logger, otlpServerFlagsCfg.HTTP); err != nil {
		return cOpts, fmt.Errorf("failed to parse OTLP/HTTP server options: %w", err)
	}
	cOpts.OTLP.HTTP.CORS = corsOTLPFlags.InitFromViper(v)
	if err := cOpts.OTLP.GRPC.initFromViper(v, logger, otlpServerFlagsCfg.GRPC); err != nil {
		return cOpts, fmt.Errorf("failed to parse OTLP/gRPC server options: %w", err)
	}

	cOpts.Zipkin.KeepAlive = v.GetBool(flagZipkinKeepAliveEnabled)
	cOpts.Zipkin.HTTPHostPort = ports.FormatHostPort(v.GetString(flagZipkinHTTPHostPort))
	tlsZipkin, err := tlsZipkinFlagsConfig.InitFromViper(v)
	if err != nil {
		return cOpts, fmt.Errorf("failed to parse Zipkin TLS options: %w", err)
	}
	cOpts.Zipkin.TLS = tlsZipkin
	cOpts.Zipkin.CORS = corsZipkinFlags.InitFromViper(v)

	return cOpts, nil
}
