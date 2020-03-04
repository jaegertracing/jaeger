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

package app

import (
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/tchannel/agent/app/reporter/tchannel"
)

// GRPCCollectorProxyBuilder creates CollectorProxyBuilder for GRPC reporter
func GRPCCollectorProxyBuilder(builder *grpc.ConnBuilder) CollectorProxyBuilder {
	return func(opts ProxyBuilderOptions) (proxy CollectorProxy, err error) {
		return grpc.NewCollectorProxy(builder, opts.AgentTags, opts.Metrics, opts.Logger)
	}
}

// TCollectorProxyBuilder creates CollectorProxyBuilder for Tchannel reporter
func TCollectorProxyBuilder(builder *tchannel.Builder) CollectorProxyBuilder {
	return func(opts ProxyBuilderOptions) (proxy CollectorProxy, err error) {
		return tchannel.NewCollectorProxy(builder, opts.Metrics, opts.Logger)
	}
}
