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

package app

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/storage"
)

const (
	queryHTTPHostPort       = "query.http-server.host-port"
	queryGRPCHostPort       = "query.grpc-server.host-port"
	queryBasePath           = "query.base-path"
	queryStaticFiles        = "query.static-files"
	queryUIConfig           = "query.ui-config"
	queryTokenPropagation   = "query.bearer-token-propagation"
	queryAdditionalHeaders  = "query.additional-headers"
	queryMaxClockSkewAdjust = "query.max-clock-skew-adjustment"
)

var tlsGRPCFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "query.grpc",
}

var tlsHTTPFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "query.http",
}

// QueryOptions holds configuration for query service
type QueryOptions struct {
	// HostPort is the host:port address that the query service listens on
	HostPort string
	// HTTPHostPort is the host:port address that the query service listens in on for http requests
	HTTPHostPort string
	// GRPCHostPort is the host:port address that the query service listens in on for gRPC requests
	GRPCHostPort string
	// BasePath is the prefix for all UI and API HTTP routes
	BasePath string
	// StaticAssets is the path for the static assets for the UI (https://github.com/uber/jaeger-ui)
	StaticAssets string
	// UIConfig is the path to a configuration file for the UI
	UIConfig string
	// BearerTokenPropagation activate/deactivate bearer token propagation to storage
	BearerTokenPropagation bool
	// TLSGRPC configures secure transport (Consumer to Query service GRPC API)
	TLSGRPC tlscfg.Options
	// TLSHTTP configures secure transport (Consumer to Query service HTTP API)
	TLSHTTP tlscfg.Options
	// AdditionalHeaders
	AdditionalHeaders http.Header
	// MaxClockSkewAdjust is the maximum duration by which jaeger-query will adjust a span
	MaxClockSkewAdjust time.Duration
}

// AddFlags adds flags for QueryOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Var(&config.StringSlice{}, queryAdditionalHeaders, `Additional HTTP response headers.  Can be specified multiple times.  Format: "Key: Value"`)
	flagSet.String(queryHTTPHostPort, ports.PortToHostPort(ports.QueryHTTP), "The host:port (e.g. 127.0.0.1:14268 or :14268) of the query's HTTP server")
	flagSet.String(queryGRPCHostPort, ports.PortToHostPort(ports.QueryGRPC), "The host:port (e.g. 127.0.0.1:14250 or :14250) of the query's gRPC server")
	flagSet.String(queryBasePath, "/", "The base path for all HTTP routes, e.g. /jaeger; useful when running behind a reverse proxy")
	flagSet.String(queryStaticFiles, "", "The directory path override for the static assets for the UI")
	flagSet.String(queryUIConfig, "", "The path to the UI configuration file in JSON format")
	flagSet.Bool(queryTokenPropagation, false, "Allow propagation of bearer token to be used by storage plugins")
	flagSet.Duration(queryMaxClockSkewAdjust, 0, "The maximum delta by which span timestamps may be adjusted in the UI due to clock skew; set to 0s to disable clock skew adjustments")
	tlsGRPCFlagsConfig.AddFlags(flagSet)
	tlsHTTPFlagsConfig.AddFlags(flagSet)
}

// InitFromViper initializes QueryOptions with properties from viper
func (qOpts *QueryOptions) InitFromViper(v *viper.Viper, logger *zap.Logger) (*QueryOptions, error) {
	qOpts.HTTPHostPort = v.GetString(queryHTTPHostPort)
	qOpts.GRPCHostPort = v.GetString(queryGRPCHostPort)
	if tlsGrpc, err := tlsGRPCFlagsConfig.InitFromViper(v); err == nil {
		qOpts.TLSGRPC = tlsGrpc
	} else {
		return qOpts, fmt.Errorf("failed to process gRPC TLS options: %w", err)
	}
	if tlsHTTP, err := tlsHTTPFlagsConfig.InitFromViper(v); err == nil {
		qOpts.TLSHTTP = tlsHTTP
	} else {
		return qOpts, fmt.Errorf("failed to process HTTP TLS options: %w", err)
	}
	qOpts.BasePath = v.GetString(queryBasePath)
	qOpts.StaticAssets = v.GetString(queryStaticFiles)
	qOpts.UIConfig = v.GetString(queryUIConfig)
	qOpts.BearerTokenPropagation = v.GetBool(queryTokenPropagation)

	qOpts.MaxClockSkewAdjust = v.GetDuration(queryMaxClockSkewAdjust)
	stringSlice := v.GetStringSlice(queryAdditionalHeaders)
	headers, err := stringSliceAsHeader(stringSlice)
	if err != nil {
		logger.Error("Failed to parse headers", zap.Strings("slice", stringSlice), zap.Error(err))
	} else {
		qOpts.AdditionalHeaders = headers
	}
	return qOpts, nil
}

// BuildQueryServiceOptions creates a QueryServiceOptions struct with appropriate adjusters and archive config
func (qOpts *QueryOptions) BuildQueryServiceOptions(storageFactory storage.Factory, logger *zap.Logger) *querysvc.QueryServiceOptions {
	opts := &querysvc.QueryServiceOptions{}
	if !opts.InitArchiveStorage(storageFactory, logger) {
		logger.Info("Archive storage not initialized")
	}

	opts.Adjuster = adjuster.Sequence(querysvc.StandardAdjusters(qOpts.MaxClockSkewAdjust)...)

	return opts
}

// stringSliceAsHeader parses a slice of strings and returns a http.Header.
//  Each string in the slice is expected to be in the format "key: value"
func stringSliceAsHeader(slice []string) (http.Header, error) {
	if len(slice) == 0 {
		return nil, nil
	}

	allHeaders := strings.Join(slice, "\r\n")

	reader := bufio.NewReader(strings.NewReader(allHeaders))
	tp := textproto.NewReader(reader)

	header, err := tp.ReadMIMEHeader()
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to parse headers")
	}

	return http.Header(header), nil
}
