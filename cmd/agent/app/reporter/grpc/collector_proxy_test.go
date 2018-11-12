// Copyright (c) 2018 The Jaeger Authors.
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

package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

func TestProxyBuilder(t *testing.T) {
	proxy := NewCollectorProxy(&Options{CollectorHostPort: []string{"localhost:0000"}}, zap.NewNop())
	require.NotNil(t, proxy)
	assert.NotNil(t, proxy.GetReporter())
	assert.NotNil(t, proxy.GetManager())
}

func TestMultipleCollectors(t *testing.T) {
	spanHandler1 := &mockSpanHandler{}
	s1, addr1 := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, spanHandler1)
	})
	defer s1.Stop()
	spanHandler2 := &mockSpanHandler{}
	s2, addr2 := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, spanHandler2)
	})
	defer s2.Stop()

	proxy := NewCollectorProxy(&Options{CollectorHostPort: []string{addr1.String(), addr2.String()}}, zap.NewNop())
	require.NotNil(t, proxy)
	assert.NotNil(t, proxy.GetReporter())
	assert.NotNil(t, proxy.GetManager())

	var bothServers = false
	// TODO do not iterate, just create two batches
	for i := 0; i < 10; i++ {
		r := proxy.GetReporter()
		err := r.EmitBatch(&jaeger.Batch{Spans: []*jaeger.Span{{OperationName: "op"}}, Process: &jaeger.Process{ServiceName: "service"}})
		require.NoError(t, err)
		if len(spanHandler1.getRequests()) > 0 && len(spanHandler2.getRequests()) > 0 {
			bothServers = true
			break
		}
	}
	assert.Equal(t, true, bothServers)
}
