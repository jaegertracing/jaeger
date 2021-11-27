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
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/proxy/app/proxysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	proxyHTTPHostPort       = "proxy.http-server.host-port"
	proxyGRPCHostPort       = "proxy.grpc-server.host-port"
	proxyBasePath           = "proxy.base-path"
	proxyStaticFiles        = "proxy.static-files"
	proxyUIConfig           = "proxy.ui-config"
	proxyTokenPropagation   = "proxy.bearer-token-propagation"
	proxyAdditionalHeaders  = "proxy.additional-headers"
	proxyMaxClockSkewAdjust = "proxy.max-clock-skew-adjustment"
	proxyQueryGRPCHostPort  = "proxy.query-grpc-server.host-port"
	proxyTagsMap            = "proxy.tags.map"
)

var tlsGRPCFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "proxy.grpc",
}

var tlsHTTPFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "proxy.http",
}

// ProxyOptions holds configuration for proxy service
type ProxyOptions struct {
	// HostPort is the host:port address that the proxy service listens on
	HostPort string
	// HTTPHostPort is the host:port address that the proxy service listens in on for http requests
	HTTPHostPort string
	// GRPCHostPort is the host:port address that the proxy service listens in on for gRPC requests
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
	// MaxClockSkewAdjust is the maximum duration by which jaeger-proxy will adjust a span
	MaxClockSkewAdjust time.Duration
	TagsMap            map[string]string
	TagsStatic         map[string]string
	QueryUpstreams     []QueryUpstream
}

type QueryUpstream struct {
	GRPCHostPort string
	Tags         model.KeyValues
}

// AddFlags adds flags for ProxyOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Var(&config.StringSlice{}, proxyAdditionalHeaders, `Additional HTTP response headers.  Can be specified multiple times.  Format: "Key: Value"`)
	flagSet.String(proxyHTTPHostPort, ports.PortToHostPort(ports.ProxyHTTP), "The host:port (e.g. 127.0.0.1:14268 or :14268) of the proxy's HTTP server")
	flagSet.String(proxyGRPCHostPort, ports.PortToHostPort(ports.ProxyGRPC), "The host:port (e.g. 127.0.0.1:14250 or :14250) of the proxy's gRPC server")
	flagSet.String(proxyBasePath, "/", "The base path for all HTTP routes, e.g. /jaeger; useful when running behind a reverse proxy")
	flagSet.String(proxyStaticFiles, "", "The directory path override for the static assets for the UI")
	flagSet.String(proxyUIConfig, "", "The path to the UI configuration file in JSON format")
	flagSet.Bool(proxyTokenPropagation, false, "Allow propagation of bearer token to be used by storage plugins")
	flagSet.Duration(proxyMaxClockSkewAdjust, 0, "The maximum delta by which span timestamps may be adjusted in the UI due to clock skew; set to 0s to disable clock skew adjustments")
	flagSet.Var(&config.StringSlice{}, proxyQueryGRPCHostPort, "The list of query gRPC servers host:port (e.g. 127.0.0.1:14250) to use as upstream")
	flagSet.Var(&config.StringSlice{}, proxyTagsMap, "The list of source:destination tag pairs to add to map on all traces")
	tlsGRPCFlagsConfig.AddFlags(flagSet)
	tlsHTTPFlagsConfig.AddFlags(flagSet)
}

// InitFromViper initializes ProxyOptions with properties from viper
func (qOpts *ProxyOptions) InitFromViper(v *viper.Viper, logger *zap.Logger) *ProxyOptions {
	qOpts.HTTPHostPort = v.GetString(proxyHTTPHostPort)
	qOpts.GRPCHostPort = v.GetString(proxyGRPCHostPort)
	qOpts.TLSGRPC = tlsGRPCFlagsConfig.InitFromViper(v)
	qOpts.TLSHTTP = tlsHTTPFlagsConfig.InitFromViper(v)
	qOpts.BasePath = v.GetString(proxyBasePath)
	qOpts.StaticAssets = v.GetString(proxyStaticFiles)
	qOpts.UIConfig = v.GetString(proxyUIConfig)
	qOpts.BearerTokenPropagation = v.GetBool(proxyTokenPropagation)

	qOpts.MaxClockSkewAdjust = v.GetDuration(proxyMaxClockSkewAdjust)
	stringSlice := v.GetStringSlice(proxyAdditionalHeaders)
	headers, err := stringSliceAsHeader(stringSlice)
	if err != nil {
		logger.Error("Failed to parse headers", zap.Strings("slice", stringSlice), zap.Error(err))
	} else {
		qOpts.AdditionalHeaders = headers
	}

	tagSlice := v.GetStringSlice(proxyTagsMap)
	tagMap, err := tagSliceAsMap(tagSlice, ":")
	if err != nil {
		logger.Warn("Invalid tag map", zap.Strings("tags", tagSlice), zap.Error(err))
	} else {
		qOpts.TagsMap = tagMap
	}

	upstreamStringSlice := v.GetStringSlice(proxyQueryGRPCHostPort)
	for _, upstream := range upstreamStringSlice {
		qOpts.QueryUpstreams = append(qOpts.QueryUpstreams, QueryUpstream{GRPCHostPort: upstream})
	}

	return qOpts
}

// BuildProxyServiceOptions creates a QueryServiceOptions struct with appropriate adjusters
func (qOpts *ProxyOptions) BuildProxyServiceOptions(logger *zap.Logger) *proxysvc.ProxyServiceOptions {
	opts := &proxysvc.ProxyServiceOptions{}

	for _, u := range qOpts.QueryUpstreams {
		upstream := proxysvc.Upstream{
			GRPCHostPort: u.GRPCHostPort,
			DialOptions:  []grpc.DialOption{grpc.WithInsecure()},
		}
		opts.Upstreams = append(opts.Upstreams, upstream)
	}

	pta := proxysvc.NewProcessTagAdjuster(qOpts.TagsMap, qOpts.TagsStatic)

	opts.Adjuster = adjuster.Sequence(proxysvc.StandardAdjusters(qOpts.MaxClockSkewAdjust, pta)...)

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

func tagSliceAsMap(slice []string, sep string) (map[string]string, error) {
	if len(slice) == 0 {
		return nil, errors.New("empty tag slice")
	}

	tagMap := make(map[string]string)
	for _, tag := range slice {
		parts := strings.Split(tag, sep)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag: %s", tag)
		} else {
			tagMap[parts[0]] = parts[1]
		}
	}
	return tagMap, nil
}
