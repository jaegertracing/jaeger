package app

import (
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

var _ api_v2.QueryServiceServer = new(GRPCHandler)
