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
	gRPCPrefix             = "reporter.grpc."
	collectorHostPort      = gRPCPrefix + "host-port"
	retry                  = gRPCPrefix + "retry.max"
	defaultMaxRetry        = 3
	collectorTLS           = gRPCPrefix + "tls"
	collectorTLSCA         = gRPCPrefix + "tls.ca"
	agentCert              = gRPCPrefix + "tls.cert"
	agentKey               = gRPCPrefix + "tls.key"
	collectorTLSServerName = gRPCPrefix + "tls.server-name"
	discoveryMinPeers      = gRPCPrefix + "discovery.min-peers"
)

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.String(collectorHostPort, "", "Comma-separated string representing host:port of a static list of collectors to connect to directly")
	flags.Uint(retry, defaultMaxRetry, "Sets the maximum number of retries for a call")
	flags.Bool(collectorTLS, false, "Use TLS when talking to the remote collector")
	flags.String(collectorTLSCA, "", "Path to a TLS CA file used to verify the remote server. (default use the systems truststore)")
	flags.String(collectorTLSServerName, "", "Override the TLS server name we expected in the remote certificate")
	flags.String(agentCert, "", "Path to a TLS client certificate file, used to identify this agent to the collector")
	flags.String(agentKey, "", "Path to the TLS client key for the client certificate")
	flags.Int(discoveryMinPeers, 3, "Max number of collectors to which the agent will try to connect at any given time")
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
	b.TLSCert = v.GetString(agentCert)
	b.TLSKey = v.GetString(agentKey)
	b.DiscoveryMinPeers = v.GetInt(discoveryMinPeers)
	return b
}
