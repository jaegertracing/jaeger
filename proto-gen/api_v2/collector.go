// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package api_v2

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
)

type PostSpansRequest = modelv1.PostSpansRequest

type PostSpansResponse = modelv1.PostSpansResponse

// CollectorServiceClient is the client API for CollectorService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type CollectorServiceClient = modelv1.CollectorServiceClient

var NewCollectorServiceClient = modelv1.NewCollectorServiceClient

// CollectorServiceServer is the server API for CollectorService service.
type CollectorServiceServer = modelv1.CollectorServiceServer

// UnimplementedCollectorServiceServer can be embedded to have forward compatible implementations.
type UnimplementedCollectorServiceServer = modelv1.UnimplementedCollectorServiceServer

var RegisterCollectorServiceServer = modelv1.RegisterCollectorServiceServer
