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

package tchannel

import (
	"flag"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	defaultConnCheckTimeout   = 250 * time.Millisecond
	tchannelPrefix            = "reporter.tchannel."
	collectorHostPort         = tchannelPrefix + "collector.host-port"
	discoveryMinPeers         = tchannelPrefix + "discovery.min-peers"
	discoveryConnCheckTimeout = tchannelPrefix + "discovery.conn-check-timeout"
)

// AddFlags adds flags for Builder.
func AddFlags(flags *flag.FlagSet) {
	flags.String(
		collectorHostPort,
		"",
		"comma-separated string representing host:ports of a static list of collectors to connect to directly (e.g. when not using service discovery)")
	flags.Int(
		discoveryMinPeers,
		defaultMinPeers,
		"if using service discovery, the min number of connections to maintain to the backend")
	flags.Duration(
		discoveryConnCheckTimeout,
		defaultConnCheckTimeout,
		"sets the timeout used when establishing new connections")
}

// InitFromViper initializes Builder with properties retrieved from Viper.
func (b *Builder) InitFromViper(v *viper.Viper) *Builder {
	if len(v.GetString(collectorHostPort)) > 0 {
		b.CollectorHostPorts = strings.Split(v.GetString(collectorHostPort), ",")
	}
	b.DiscoveryMinPeers = v.GetInt(discoveryMinPeers)
	b.ConnCheckTimeout = v.GetDuration(discoveryConnCheckTimeout)
	return b
}
