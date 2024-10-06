// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func GetValidBearerToken(ctx context.Context) (string, error) {
	bearerToken, ok := GetBearerToken(ctx)
	if ok && bearerToken != "" {
		return bearerToken, nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	}

	var err error
	bearerHeader := "bearer"
	bearerToken, err = TokenFromMetadata(md, bearerHeader)
	if err != nil {
		return "", err
	}
	if bearerToken == "" {
		return bearerToken, status.Errorf(codes.PermissionDenied, "unknown tenant")
	}

	return bearerToken, nil
}

func TokenFromMetadata(md metadata.MD, bearerHeader string) (string, error) {
	bearerToken := md.Get(bearerHeader)
	if len(bearerToken) < 1 {
		return "", status.Errorf(codes.Unauthenticated, "missing tenant header")
	} else if len(bearerToken) > 1 {
		return "", status.Errorf(codes.PermissionDenied, "extra tenant header")
	}

	return bearerToken[0], nil
}
