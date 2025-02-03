// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	v2querysvc "github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/v1adapter"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	queryHTTPHostPort          = "query.http-server.host-port"
	queryGRPCHostPort          = "query.grpc-server.host-port"
	queryBasePath              = "query.base-path"
	queryStaticFiles           = "query.static-files"
	queryLogStaticAssetsAccess = "query.log-static-assets-access"
	queryUIConfig              = "query.ui-config"
	queryTokenPropagation      = "query.bearer-token-propagation"
	queryAdditionalHeaders     = "query.additional-headers"
	queryMaxClockSkewAdjust    = "query.max-clock-skew-adjustment"
	queryEnableTracing         = "query.enable-tracing"
)

const (
	defaultMaxClockSkewAdjust = 0 * time.Second
)

var tlsGRPCFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "query.grpc",
}

var tlsHTTPFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "query.http",
}

type UIConfig struct {
	// ConfigFile is the path to a configuration file for the UI.
	ConfigFile string `mapstructure:"config_file" valid:"optional"`
	// AssetsPath is the path for the static assets for the UI (https://github.com/uber/jaeger-ui).
	AssetsPath string `mapstructure:"assets_path" valid:"optional" `
	// LogAccess tells static handler to log access to static assets, useful in debugging.
	LogAccess bool `mapstructure:"log_access" valid:"optional"`
}

// QueryOptions holds configuration for query service shared with jaeger-v2
type QueryOptions struct {
	// BasePath is the base path for all HTTP routes.
	BasePath string `mapstructure:"base_path"`
	// UIConfig contains configuration related to the Jaeger UIConfig.
	UIConfig UIConfig `mapstructure:"ui"`
	// BearerTokenPropagation activate/deactivate bearer token propagation to storage.
	BearerTokenPropagation bool `mapstructure:"bearer_token_propagation"`
	// Tenancy holds the multi-tenancy configuration.
	Tenancy tenancy.Options `mapstructure:"multi_tenancy"`
	// MaxClockSkewAdjust is the maximum duration by which jaeger-query will adjust a span.
	MaxClockSkewAdjust time.Duration `mapstructure:"max_clock_skew_adjust"  valid:"optional"`
	// EnableTracing determines whether traces will be emitted by jaeger-query.
	EnableTracing bool `mapstructure:"enable_tracing"`
	// HTTP holds the HTTP configuration that the query service uses to serve requests.
	HTTP confighttp.ServerConfig `mapstructure:"http"`
	// GRPC holds the GRPC configuration that the query service uses to serve requests.
	GRPC configgrpc.ServerConfig `mapstructure:"grpc"`
}

// AddFlags adds flags for QueryOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Var(&config.StringSlice{}, queryAdditionalHeaders, `Additional HTTP response headers.  Can be specified multiple times.  Format: "Key: Value"`)
	flagSet.String(queryHTTPHostPort, ports.PortToHostPort(ports.QueryHTTP), "The host:port (e.g. 127.0.0.1:14268 or :14268) of the query's HTTP server")
	flagSet.String(queryGRPCHostPort, ports.PortToHostPort(ports.QueryGRPC), "The host:port (e.g. 127.0.0.1:14250 or :14250) of the query's gRPC server")
	flagSet.String(queryBasePath, "/", "The base path for all HTTP routes, e.g. /jaeger; useful when running behind a reverse proxy. See https://github.com/jaegertracing/jaeger/blob/main/examples/reverse-proxy/README.md")
	flagSet.String(queryStaticFiles, "", "The directory path override for the static assets for the UI")
	flagSet.Bool(queryLogStaticAssetsAccess, false, "Log when static assets are accessed (for debugging)")
	flagSet.String(queryUIConfig, "", "The path to the UI configuration file in JSON format")
	flagSet.Bool(queryTokenPropagation, false, "Allow propagation of bearer token to be used by storage plugins")
	flagSet.Duration(queryMaxClockSkewAdjust, defaultMaxClockSkewAdjust, "The maximum delta by which span timestamps may be adjusted in the UI due to clock skew; set to 0s to disable clock skew adjustments")
	flagSet.Bool(queryEnableTracing, false, "Enables emitting jaeger-query traces")
	tlsGRPCFlagsConfig.AddFlags(flagSet)
	tlsHTTPFlagsConfig.AddFlags(flagSet)
}

// InitFromViper initializes QueryOptions with properties from viper
func (qOpts *QueryOptions) InitFromViper(v *viper.Viper, logger *zap.Logger) (*QueryOptions, error) {
	qOpts.HTTP.Endpoint = v.GetString(queryHTTPHostPort)
	qOpts.GRPC.NetAddr.Endpoint = v.GetString(queryGRPCHostPort)
	// TODO: drop support for same host ports
	// https://github.com/jaegertracing/jaeger/issues/6117
	if qOpts.HTTP.Endpoint == qOpts.GRPC.NetAddr.Endpoint {
		logger.Warn("using the same port for gRPC and HTTP is deprecated; please use dedicated ports instead; support for shared ports will be removed in Feb 2025")
	}
	tlsGrpc, err := tlsGRPCFlagsConfig.InitFromViper(v)
	if err != nil {
		return qOpts, fmt.Errorf("failed to process gRPC TLS options: %w", err)
	}
	qOpts.GRPC.TLSSetting = tlsGrpc
	tlsHTTP, err := tlsHTTPFlagsConfig.InitFromViper(v)
	if err != nil {
		return qOpts, fmt.Errorf("failed to process HTTP TLS options: %w", err)
	}
	qOpts.HTTP.TLSSetting = tlsHTTP
	qOpts.BasePath = v.GetString(queryBasePath)
	qOpts.UIConfig.AssetsPath = v.GetString(queryStaticFiles)
	qOpts.UIConfig.LogAccess = v.GetBool(queryLogStaticAssetsAccess)
	qOpts.UIConfig.ConfigFile = v.GetString(queryUIConfig)
	qOpts.BearerTokenPropagation = v.GetBool(queryTokenPropagation)

	qOpts.MaxClockSkewAdjust = v.GetDuration(queryMaxClockSkewAdjust)
	stringSlice := v.GetStringSlice(queryAdditionalHeaders)
	headers, err := stringSliceAsHeader(stringSlice)
	if err != nil {
		logger.Error("Failed to parse headers", zap.Strings("slice", stringSlice), zap.Error(err))
	} else {
		qOpts.HTTP.ResponseHeaders = mapHTTPHeaderToOTELHeaders(headers)
	}
	qOpts.Tenancy = tenancy.InitFromViper(v)
	qOpts.EnableTracing = v.GetBool(queryEnableTracing)
	return qOpts, nil
}

type InitArchiveStorageFn func() (*storage.ArchiveStorage, error)

// BuildQueryServiceOptions creates a QueryServiceOptions struct with appropriate adjusters and archive config
func (qOpts *QueryOptions) BuildQueryServiceOptions(
	initArchiveStorageFn InitArchiveStorageFn,
	logger *zap.Logger,
) (*querysvc.QueryServiceOptions, *v2querysvc.QueryServiceOptions) {
	opts := &querysvc.QueryServiceOptions{
		MaxClockSkewAdjust: qOpts.MaxClockSkewAdjust,
	}
	v2Opts := &v2querysvc.QueryServiceOptions{
		MaxClockSkewAdjust: qOpts.MaxClockSkewAdjust,
	}
	as, err := initArchiveStorageFn()
	if err != nil {
		logger.Error("Received an error when trying to initialize archive storage", zap.Error(err))
		return opts, v2Opts
	}

	if as != nil && as.Reader != nil && as.Writer != nil {
		opts.ArchiveSpanReader = as.Reader
		opts.ArchiveSpanWriter = as.Writer
		v2Opts.ArchiveTraceReader = v1adapter.NewTraceReader(as.Reader)
		v2Opts.ArchiveTraceWriter = v1adapter.NewTraceWriter(as.Writer)
	} else {
		logger.Info("Archive storage not initialized")
	}

	return opts, v2Opts
}

// stringSliceAsHeader parses a slice of strings and returns a http.Header.
// Each string in the slice is expected to be in the format "key: value"
func stringSliceAsHeader(slice []string) (http.Header, error) {
	if len(slice) == 0 {
		return nil, nil
	}

	allHeaders := strings.Join(slice, "\r\n")

	reader := bufio.NewReader(strings.NewReader(allHeaders))
	tp := textproto.NewReader(reader)

	header, err := tp.ReadMIMEHeader()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, errors.New("failed to parse headers")
	}

	return http.Header(header), nil
}

func mapHTTPHeaderToOTELHeaders(h http.Header) map[string]configopaque.String {
	otelHeaders := make(map[string]configopaque.String)
	for key, values := range h {
		otelHeaders[key] = configopaque.String(strings.Join(values, ","))
	}

	return otelHeaders
}

func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		MaxClockSkewAdjust: defaultMaxClockSkewAdjust,
		HTTP: confighttp.ServerConfig{
			Endpoint: ports.PortToHostPort(ports.QueryHTTP),
		},
		GRPC: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.QueryGRPC),
				Transport: confignet.TransportTypeTCP,
			},
		},
	}
}
