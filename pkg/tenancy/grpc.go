// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tenancy

import (
	"context"

	"go.opentelemetry.io/collector/client"
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

func GetValidTenant(ctx context.Context, tm *Manager) (string, error) {
	tenant, err := extractTenantFromSources(ctx, tm.Header)
	if err != nil {
		return "", err
	}

	if !tm.Valid(tenant) {
		return "", status.Errorf(codes.PermissionDenied, "unknown tenant")
	}

	return tenant, nil
}

// helper function to extract tenant from different sources
func extractTenantFromSources(ctx context.Context, header string) (string, error) {
	if tenant := GetTenant(ctx); tenant != "" {
		return tenant, nil
	}

	if cli := client.FromContext(ctx); cli.Metadata.Get(header) != nil {
		if tenants := cli.Metadata.Get(header); len(tenants) > 0 {
			return extractSingleTenant(tenants)
		}
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	}

	return extractSingleTenant(md.Get(header))
}

// Helper function for metadata extraction
func tenantFromMetadata(md metadata.MD, header string) (string, error) {
	tenants := md.Get(header)
	return extractSingleTenant(tenants)
}

// Ensures single tenant value exists
func extractSingleTenant(tenants []string) (string, error) {
	switch len(tenants) {
	case 0:
		return "", status.Errorf(codes.Unauthenticated, "missing tenant header")
	case 1:
		return tenants[0], nil
	default:
		return "", status.Errorf(codes.PermissionDenied, "extra tenant header")
	}
}

func directlyAttachedTenant(ctx context.Context) bool {
	return GetTenant(ctx) != ""
}

// NewGuardingStreamInterceptor blocks handling of streams whose tenancy header doesn't meet tenancy requirements.
// It also ensures the tenant is directly in the context, rather than context metadata.
func NewGuardingStreamInterceptor(tc *Manager) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		tenant, err := GetValidTenant(ss.Context(), tc)
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

// NewGuardingUnaryInterceptor blocks handling of RPCs whose tenancy header doesn't meet tenancy requirements.
// It also ensures the tenant is directly in the context, rather than context metadata.
func NewGuardingUnaryInterceptor(tc *Manager) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		tenant, err := GetValidTenant(ctx, tc)
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
		req, reply any,
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

// NewClientStreamInterceptor injects tenant header into gRPC request metadata.
func NewClientStreamInterceptor(tc *Manager) grpc.StreamClientInterceptor {
	return grpc.StreamClientInterceptor(func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if tenant := GetTenant(ctx); tenant != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, tc.Header, tenant)
		}
		return streamer(ctx, desc, cc, method, opts...)
	})
}
