// Copyright (c) 2020 The Jaeger Authors.
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

package jaegerreceiver

import (
	"flag"
	"strconv"

	agentApp "github.com/jaegertracing/jaeger/cmd/agent/app"
	grpcRep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	thriftBinaryHostPort  = "processor.jaeger-binary.server-host-port"
	thriftCompactHostPort = "processor.jaeger-compact.server-host-port"
)

// AddFlags adds flags to flag set.
func AddFlags(flags *flag.FlagSet) {
	flags.String(thriftBinaryHostPort, ":"+strconv.Itoa(ports.AgentJaegerThriftBinaryUDP), "host:port for the UDP server")
	flags.String(thriftCompactHostPort, ":"+strconv.Itoa(ports.AgentJaegerThriftCompactUDP), "host:port for the UDP server")
	collectorApp.AddOTELJaegerFlags(flags)
	agentApp.AddOTELFlags(flags)
	grpcRep.AddOTELFlags(flags)
	static.AddOTELFlags(flags)
}
