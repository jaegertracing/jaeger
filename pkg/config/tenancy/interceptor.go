// Copyright (c) 2022 The Jaeger Authors.
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

package tenancy

import (
	"context"

	"google.golang.org/grpc"
)

// tenantedServerStream is a wrapper for ServerStream providing settable context
type tenantedServerStream struct {
	grpc.ServerStream
	context context.Context
}

func (tss *tenantedServerStream) Context() context.Context {
	return tss.context
}

// NewGuardingStreamInterceptor blocks handling of streams whose tenancy header isn't doesn't meet tenancy requirements
func NewGuardingStreamInterceptor(tc *TenancyConfig) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		tenantedCtx, err := tc.GetValidTenantContext(ss.Context())
		if err != nil {
			return err
		}

		wrappedSS := &tenantedServerStream{
			ServerStream: ss,
			context:      tenantedCtx,
		}
		return handler(srv, wrappedSS)
	}
}
