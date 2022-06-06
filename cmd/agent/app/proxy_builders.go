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
	"google.golang.org/grpc/metadata"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
)

// GRPCCollectorProxyBuilder creates CollectorProxyBuilder for GRPC reporter
func GRPCCollectorProxyBuilder(builder *grpc.ConnBuilder) CollectorProxyBuilder {
	return func(opts ProxyBuilderOptions) (proxy CollectorProxy, err error) {
		md := metadata.New(map[string]string{})
		if opts.TenancyHeader != "" {
			md = metadata.New(map[string]string{opts.TenancyHeader: opts.Tenant})
		}
		return grpc.NewCollectorProxy(builder, opts.AgentTags, opts.Metrics, md, opts.Logger)
	}
}
