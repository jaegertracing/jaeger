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

package builder

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	queryPort                = "query.port"
	queryPrefix              = "query.prefix"
	queryStaticFiles         = "query.static-files"
	queryHealthCheckHTTPPort = "query.health-check-http-port"
)

// QueryOptions holds configuration for query
type QueryOptions struct {
	// QueryPort is the port that the query service listens in on
	QueryPort int
	// QueryPrefix is the prefix of the query service api
	QueryPrefix string
	// QueryStaticAssets is the path for the static assets for the UI (https://github.com/uber/jaeger-ui)
	QueryStaticAssets string
	// QueryHealthCheckHTTPPort is the port that the health check service listens in on for http requests
	QueryHealthCheckHTTPPort int
}

// AddFlags adds flags for QueryOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(queryPort, 16686, "The port for the query service")
	flagSet.String(queryPrefix, "api", "The prefix for the url of the query service")
	flagSet.String(queryStaticFiles, "jaeger-ui-build/build/", "The path for the static assets for the UI")
	flagSet.Int(queryHealthCheckHTTPPort, 16687, "The http port for the health check service")
}

// InitFromViper initializes QueryOptions with properties from viper
func (qOpts *QueryOptions) InitFromViper(v *viper.Viper) *QueryOptions {
	qOpts.QueryPort = v.GetInt(queryPort)
	qOpts.QueryPrefix = v.GetString(queryPrefix)
	qOpts.QueryStaticAssets = v.GetString(queryStaticFiles)
	qOpts.QueryHealthCheckHTTPPort = v.GetInt(queryHealthCheckHTTPPort)
	return qOpts
}
