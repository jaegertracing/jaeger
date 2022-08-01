// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package flags

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/flags"
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

	flagSuffixGRPCMaxReceiveMessageLength = "max-message-size"
	flagSuffixGRPCMaxConnectionAge        = "max-connection-age"
	flagSuffixGRPCMaxConnectionAgeGrace   = "max-connection-age-grace"

	flagCollectorOTLPEnabled = "collector.otlp.enabled"

	flagZipkinHTTPHostPort   = "collector.zipkin.host-port"
	flagZipkinAllowedHeaders = "collector.zipkin.allowed-headers"
	flagZipkinAllowedOrigins = "collector.zipkin.allowed-origins"

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
			Prefix: "collector.otlp.grpc",
		},
	},
	HTTP: serverFlagsConfig{
		prefix: "collector.otlp.http",
		tls: tlscfg.ServerFlagsConfig{
			Prefix: "collector.otlp.http",
		},
	},
}

var tlsZipkinFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "collector.zipkin",
}

// CollectorOptions holds configuration for collector
type CollectorOptions struct {
	// DynQueueSizeMemory determines how much memory to use for the queue
	DynQueueSizeMemory uint
	// QueueSize is the size of collector's queue
	QueueSize int
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
		// ZipkinAllowedOrigins is a list of origins a cross-domain request to the Zipkin collector service can be executed from
		AllowedOrigins string
		// ZipkinAllowedHeaders is a list of headers that the Zipkin collector service allowes the client to use with cross-domain requests
		AllowedHeaders string
		// TLS configures secure transport for Zipkin endpoint to collect spans
		TLS tlscfg.Options
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

	flags.Bool(flagCollectorOTLPEnabled, false, "Enables OpenTelemetry OTLP receiver on dedicated HTTP and gRPC ports")
	addHTTPFlags(flags, otlpServerFlagsCfg.HTTP, "")
	addGRPCFlags(flags, otlpServerFlagsCfg.GRPC, "")

	flags.String(flagZipkinAllowedHeaders, "content-type", "Comma separated list of allowed headers for the Zipkin collector service, default content-type")
	flags.String(flagZipkinAllowedOrigins, "*", "Comma separated list of allowed origins for the Zipkin collector service, default accepts all")
	flags.String(flagZipkinHTTPHostPort, "", "The host:port (e.g. 127.0.0.1:9411 or :9411) of the collector's Zipkin server (disabled by default)")
	tlsZipkinFlagsConfig.AddFlags(flags)

	tenancy.AddFlags(flags)
}

func addHTTPFlags(flags *flag.FlagSet, cfg serverFlagsConfig, defaultHostPort string) {
	flags.String(cfg.prefix+"."+flagSuffixHostPort, defaultHostPort, "The host:port (e.g. 127.0.0.1:12345 or :12345) of the collector's HTTP server")
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

func (opts *HTTPOptions) initFromViper(v *viper.Viper, logger *zap.Logger, cfg serverFlagsConfig) error {
	opts.HostPort = ports.FormatHostPort(v.GetString(cfg.prefix + "." + flagSuffixHostPort))
	if tlsOpts, err := cfg.tls.InitFromViper(v); err == nil {
		opts.TLS = tlsOpts
	} else {
		return fmt.Errorf("failed to parse HTTP TLS options: %w", err)
	}
	return nil
}

func (opts *GRPCOptions) initFromViper(v *viper.Viper, logger *zap.Logger, cfg serverFlagsConfig) error {
	opts.HostPort = ports.FormatHostPort(v.GetString(cfg.prefix + "." + flagSuffixHostPort))
	opts.MaxReceiveMessageLength = v.GetInt(cfg.prefix + "." + flagSuffixGRPCMaxReceiveMessageLength)
	opts.MaxConnectionAge = v.GetDuration(cfg.prefix + "." + flagSuffixGRPCMaxConnectionAge)
	opts.MaxConnectionAgeGrace = v.GetDuration(cfg.prefix + "." + flagSuffixGRPCMaxConnectionAgeGrace)
	if tlsOpts, err := cfg.tls.InitFromViper(v); err == nil {
		opts.TLS = tlsOpts
	} else {
		return fmt.Errorf("failed to parse gRPC TLS options: %w", err)
	}
	opts.Tenancy = tenancy.InitFromViper(v)

	return nil
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *CollectorOptions) InitFromViper(v *viper.Viper, logger *zap.Logger) (*CollectorOptions, error) {
	cOpts.CollectorTags = flags.ParseJaegerTags(v.GetString(flagCollectorTags))
	cOpts.NumWorkers = v.GetInt(flagNumWorkers)
	cOpts.QueueSize = v.GetInt(flagQueueSize)
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
	if err := cOpts.OTLP.GRPC.initFromViper(v, logger, otlpServerFlagsCfg.GRPC); err != nil {
		return cOpts, fmt.Errorf("failed to parse OTLP/gRPC server options: %w", err)
	}

	cOpts.Zipkin.AllowedHeaders = v.GetString(flagZipkinAllowedHeaders)
	cOpts.Zipkin.AllowedOrigins = v.GetString(flagZipkinAllowedOrigins)
	cOpts.Zipkin.HTTPHostPort = ports.FormatHostPort(v.GetString(flagZipkinHTTPHostPort))
	if tlsZipkin, err := tlsZipkinFlagsConfig.InitFromViper(v); err == nil {
		cOpts.Zipkin.TLS = tlsZipkin
	} else {
		return cOpts, fmt.Errorf("failed to parse Zipkin TLS options: %w", err)
	}

	return cOpts, nil
}
