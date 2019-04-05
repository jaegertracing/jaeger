// Copyright (c) 2018 The Jaeger Authors.
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

package grpc

import (
	"flag"
	"strings"

	"github.com/spf13/viper"
)

const (
	gRPCPrefix              = "reporter.grpc."
	collectorHostPort       = gRPCPrefix + "host-port"
	retry                   = gRPCPrefix + "retry.max"
	defaultMaxRetry         = 3
	collectorTLS            = gRPCPrefix + "tls"
	collectorTLSCA          = gRPCPrefix + "tls.ca"
	collectorTLSServerName  = gRPCPrefix + "tls.server-name"
	collectorResolverTarget = gRPCPrefix + "resolver.target"
)

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.String(collectorHostPort, "", "Comma-separated string representing host:port of a static list of collectors to connect to directly.")
	flags.Uint(retry, defaultMaxRetry, "Sets the maximum number of retries for a call.")
	flags.Bool(collectorTLS, false, "Enable TLS.")
	flags.String(collectorTLSCA, "", "Path to a TLS CA file. (default use the systems truststore)")
	flags.String(collectorTLSServerName, "", "Override the TLS server name.")
	flags.String(collectorResolverTarget, "", "Path to collector resolver")
}

// InitFromViper initializes Options with properties retrieved from Viper.
func (b *ConnBuilder) InitFromViper(v *viper.Viper) *ConnBuilder {
	hostPorts := v.GetString(collectorHostPort)
	if hostPorts != "" {
		b.CollectorHostPorts = strings.Split(hostPorts, ",")
	}
	b.MaxRetry = uint(v.GetInt(retry))
	b.TLS = v.GetBool(collectorTLS)
	b.TLSCA = v.GetString(collectorTLSCA)
	b.TLSServerName = v.GetString(collectorTLSServerName)
	b.ResolverTarget = v.GetString(collectorResolverTarget)
	return b
}
