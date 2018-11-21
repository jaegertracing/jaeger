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
	"flag"
	"time"

	"github.com/spf13/viper"
)

const (
	queryPort                 = "query.port"
	queryBasePath             = "query.base-path"
	queryStaticFiles          = "query.static-files"
	queryUIConfig             = "query.ui-config"
	queryCacheRefreshInterval = "query.cache-refresh-interval"
	// QueryDefaultHealthCheckHTTPPort is the default HTTP Port for health check
	QueryDefaultHealthCheckHTTPPort = 16687
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
	// CacheRefreshInterval is the refresh interval for read cache
	CacheRefreshInterval time.Duration
}

// AddFlags adds flags for QueryOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(queryPort, 16686, "The port for the query service")
	flagSet.String(queryBasePath, "/", "The base path for all HTTP routes, e.g. /jaeger; useful when running behind a reverse proxy")
	flagSet.String(queryStaticFiles, "", "The directory path override for the static assets for the UI")
	flagSet.String(queryUIConfig, "", "The path to the UI configuration file in JSON format")
	flagSet.Duration(queryCacheRefreshInterval, 2*time.Minute, "The duration between cache refreshes")
}

// InitFromViper initializes QueryOptions with properties from viper
func (qOpts *QueryOptions) InitFromViper(v *viper.Viper) *QueryOptions {
	qOpts.Port = v.GetInt(queryPort)
	qOpts.BasePath = v.GetString(queryBasePath)
	qOpts.StaticAssets = v.GetString(queryStaticFiles)
	qOpts.UIConfig = v.GetString(queryUIConfig)
	qOpts.CacheRefreshInterval = v.GetDuration(queryCacheRefreshInterval)
	return qOpts
}
