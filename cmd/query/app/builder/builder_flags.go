// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package builder

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	queryPort        = "query.port"
	queryPrefix      = "query.prefix"
	queryStaticFiles = "query.static-files"
)

// QueryOptions holds configuration for query
type QueryOptions struct {
	// QueryPort is the port that the query service listens in on
	QueryPort int
	// QueryPrefix is the prefix of the query service api
	QueryPrefix string
	// QueryStaticAssets is the path for the static assets for the UI (https://github.com/uber/jaeger-ui)
	QueryStaticAssets string
}

// AddFlags adds flags for QueryOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(queryPort, 16686, "The port for the query service")
	flagSet.String(queryPrefix, "api", "The prefix for the url of the query service")
	flagSet.String(queryStaticFiles, "jaeger-ui-build/build/", "The path for the static assets for the UI")
}

// InitFromViper initializes QueryOptions with properties from viper
func (qOpts *QueryOptions) InitFromViper(v *viper.Viper) *QueryOptions {
	qOpts.QueryPort = v.GetInt(queryPort)
	qOpts.QueryPrefix = v.GetString(queryPrefix)
	qOpts.QueryStaticAssets = v.GetString(queryStaticFiles)
	return qOpts
}
