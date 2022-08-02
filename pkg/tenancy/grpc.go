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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// tenantedServerStream is a wrapper for ServerStream providing settable context
type tenantedServerStream struct {
	grpc.ServerStream
	context context.Context
}

func (tss *tenantedServerStream) Context() context.Context {
	return tss.context
}

func getValidTenant(ctx context.Context, tc *Manager) (string, error) {
	// Handle case where tenant is already directly in the context
	tenant := GetTenant(ctx)
	if tenant != "" {
		if !tc.Valid(tenant) {
			return tenant, status.Errorf(codes.PermissionDenied, "unknown tenant")
		}
		return tenant, nil
	}

	// Handle case where tenant is in the context metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	}

	var err error
	tenant, err = tenantFromMetadata(md, tc.Header)
	if err != nil {
		return "", err
	}
	if !tc.Valid(tenant) {
		return tenant, status.Errorf(codes.PermissionDenied, "unknown tenant")
	}

	return tenant, nil
}

func directlyAttachedTenant(ctx context.Context) bool {
	return GetTenant(ctx) != ""
}

// NewGuardingStreamInterceptor blocks handling of streams whose tenancy header doesn't meet tenancy requirements.
// It also ensures the tenant is directly in the context, rather than context metadata.
func NewGuardingStreamInterceptor(tc *Manager) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		tenant, err := getValidTenant(ss.Context(), tc)
		if err != nil {
			return err
		}

		if directlyAttachedTenant(ss.Context()) {
			return handler(srv, ss)
		}

		// "upgrade" the tenant to be part of the context, rather than just incoming metadata
		return handler(srv, &tenantedServerStream{
			ServerStream: ss,
			context:      WithTenant(ss.Context(), tenant),
		})
	}
}

func tenantFromMetadata(md metadata.MD, tenancyHeader string) (string, error) {
	tenants := md.Get(tenancyHeader)
	if len(tenants) < 1 {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	} else if len(tenants) > 1 {
		return "", status.Errorf(codes.PermissionDenied, "extra tenant header")
	}

	return tenants[0], nil
}

// NewGuardingUnaryInterceptor blocks handling of RPCs whose tenancy header doesn't meet tenancy requirements.
// It also ensures the tenant is directly in the context, rather than context metadata.
func NewGuardingUnaryInterceptor(tc *Manager) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		tenant, err := getValidTenant(ctx, tc)
		if err != nil {
			return nil, err
		}

		if directlyAttachedTenant(ctx) {
			return handler(ctx, req)
		}

		return handler(WithTenant(ctx, tenant), req)
	}
}

// NewClientUnaryInterceptor injects tenant header into gRPC request metadata.
func NewClientUnaryInterceptor(tc *Manager) grpc.UnaryClientInterceptor {
	return grpc.UnaryClientInterceptor(func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if tenant := GetTenant(ctx); tenant != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, tc.Header, tenant)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	})
}
