package bearertoken

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
)

type tokenatedServerStream struct {
	grpc.ServerStream
	context context.Context
}

func (tss *tokenatedServerStream) Context() context.Context {
	return tss.context
}

// getValidBearerToken attempts to retrieve the bearer token from the context.
// It does not return an error if the token is missing.
func getValidBearerToken(ctx context.Context, bearerHeader string) (string, error) {
	bearerToken, ok := GetBearerToken(ctx)
	if ok && bearerToken != "" {
		return bearerToken, nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", nil
	}

	bearerToken, err := tokenFromMetadata(md, bearerHeader)
	if err != nil {
		return "", err
	}

	return bearerToken, nil
}

// tokenFromMetadata extracts the bearer token from the metadata.
func tokenFromMetadata(md metadata.MD, bearerHeader string) (string, error) {
	bearerToken := md.Get(bearerHeader)
	if len(bearerToken) < 1 {
		return "", nil
	}
	if len(bearerToken) > 1 {
		return "", status.Errorf(codes.PermissionDenied, "extra bearer token header")
	}
	return bearerToken[0], nil
}

// directlyAttachedBearerToken checks if the bearer token is directly attached to the context.
func directlyAttachedBearerToken(ctx context.Context) bool {
	bearerToken, _ := GetBearerToken(ctx)
	return bearerToken != ""
}

// NewGuardingStreamInterceptor creates a new stream interceptor that injects the bearer token into the context if available.
func NewGuardingStreamInterceptor(bt string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		bearerToken, err := getValidBearerToken(ss.Context(), bt)
		if err != nil {
			return err
		}

		if directlyAttachedBearerToken(ss.Context()) {
			return handler(srv, ss)
		}

		// Upgrade the bearer token to be part of the context.
		return handler(srv, &tokenatedServerStream{
			ServerStream: ss,
			context:      ContextWithBearerToken(ss.Context(), bearerToken),
		})
	}
}

// NewGuardingUnaryInterceptor creates a new unary interceptor that injects the bearer token into the context if available.
func NewGuardingUnaryInterceptor(bt string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		bearerToken, err := getValidBearerToken(ctx, bt)
		if err != nil {
			return nil, err
		}

		if directlyAttachedBearerToken(ctx) {
			return handler(ctx, req)
		}

		return handler(ContextWithBearerToken(ctx, bearerToken), req)
	}
}

// NewClientUnaryInterceptor injects the bearer token header into gRPC request metadata.
func NewClientUnaryInterceptor(bt string) grpc.UnaryClientInterceptor {
	return grpc.UnaryClientInterceptor(func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if bearerToken, _ := GetBearerToken(ctx); bearerToken != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, bt, bearerToken)
		}
		// Proceed with the request even if no token is found
		return invoker(ctx, method, req, reply, cc, opts...)
	})
}

// NewClientStreamInterceptor injects the bearer token header into gRPC request metadata.
func NewClientStreamInterceptor(bt string) grpc.StreamClientInterceptor {
	return grpc.StreamClientInterceptor(func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if bearerToken, _ := GetBearerToken(ctx); bearerToken != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, bt, bearerToken)
		}
		// Proceed with the request even if no token is found
		return streamer(ctx, desc, cc, method, opts...)
	})
}
