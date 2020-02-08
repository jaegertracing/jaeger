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

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	queryPort              = "query.port"
	queryBasePath          = "query.base-path"
	queryStaticFiles       = "query.static-files"
	queryUIConfig          = "query.ui-config"
	queryTokenPropagation  = "query.bearer-token-propagation"
	queryAdditionalHeaders = "query.additional-headers"
)

// QueryOptions holds configuration for query service
type QueryOptions struct {
	// Port is the port that the query service listens in on
	Port int
	// BasePath is the prefix for all UI and API HTTP routes
	BasePath string
	// StaticAssets is the path for the static assets for the UI (https://github.com/uber/jaeger-ui)
	StaticAssets string
	// UIConfig is the path to a configuration file for the UI
	UIConfig string
	// BearerTokenPropagation activate/deactivate bearer token propagation to storage
	BearerTokenPropagation bool
	// AdditionalHeaders
	AdditionalHeaders http.Header
}

// AddFlags adds flags for QueryOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Var(&config.StringSlice{}, queryAdditionalHeaders, `Additional HTTP response headers.  Can be specified multiple times.  Format: "Key: Value"`)
	flagSet.Int(queryPort, ports.QueryHTTP, "The port for the query service")
	flagSet.String(queryBasePath, "/", "The base path for all HTTP routes, e.g. /jaeger; useful when running behind a reverse proxy")
	flagSet.String(queryStaticFiles, "", "The directory path override for the static assets for the UI")
	flagSet.String(queryUIConfig, "", "The path to the UI configuration file in JSON format")
	flagSet.Bool(queryTokenPropagation, false, "Allow propagation of bearer token to be used by storage plugins")
}

// InitFromViper initializes QueryOptions with properties from viper
func (qOpts *QueryOptions) InitFromViper(v *viper.Viper, logger *zap.Logger) *QueryOptions {
	qOpts.Port = v.GetInt(queryPort)
	qOpts.BasePath = v.GetString(queryBasePath)
	qOpts.StaticAssets = v.GetString(queryStaticFiles)
	qOpts.UIConfig = v.GetString(queryUIConfig)
	qOpts.BearerTokenPropagation = v.GetBool(queryTokenPropagation)

	stringSlice := v.GetStringSlice(queryAdditionalHeaders)
	headers, err := stringSliceAsHeader(stringSlice)
	if err != nil {
		logger.Error("Failed to parse headers", zap.Strings("slice", stringSlice), zap.Error(err))
	} else {
		qOpts.AdditionalHeaders = headers
	}
	return qOpts
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
