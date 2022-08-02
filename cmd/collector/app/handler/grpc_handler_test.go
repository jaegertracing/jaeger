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

package handler

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

type mockSpanProcessor struct {
	expectedError error
	mux           sync.Mutex
	spans         []*model.Span
	tenants       map[string]bool
}

func (p *mockSpanProcessor) ProcessSpans(spans []*model.Span, opts processor.SpansOptions) ([]bool, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.spans = append(p.spans, spans...)
	oks := make([]bool, len(spans))
	if p.tenants == nil {
		p.tenants = make(map[string]bool)
	}
	p.tenants[opts.Tenant] = true
	return oks, p.expectedError
}

func (p *mockSpanProcessor) getSpans() []*model.Span {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.spans
}

func (p *mockSpanProcessor) getTenants() map[string]bool {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.tenants
}

func (p *mockSpanProcessor) reset() {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.spans = nil
	p.tenants = nil
}

func (p *mockSpanProcessor) Close() error {
	return nil
}

func initializeGRPCTestServer(t *testing.T, beforeServe func(s *grpc.Server)) (*grpc.Server, net.Addr) {
	server := grpc.NewServer()
	beforeServe(server)
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	go func() {
		err := server.Serve(lis)
		require.NoError(t, err)
	}()
	return server, lis.Addr()
}

func newClient(t *testing.T, addr net.Addr) (api_v2.CollectorServiceClient, *grpc.ClientConn) {
	conn, err := grpc.Dial(addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	return api_v2.NewCollectorServiceClient(conn), conn
}

func TestPostSpans(t *testing.T) {
	processor := &mockSpanProcessor{}
	server, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		handler := NewGRPCHandler(zap.NewNop(), processor, &tenancy.Manager{})
		api_v2.RegisterCollectorServiceServer(s, handler)
	})
	defer server.Stop()
	client, conn := newClient(t, addr)
	defer conn.Close()

	tests := []struct {
		batch    model.Batch
		expected []*model.Span
	}{
		{
			batch:    model.Batch{Process: &model.Process{ServiceName: "batch-process"}, Spans: []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "bar"}}}},
			expected: []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "bar"}}},
		},
		{
			batch:    model.Batch{Process: &model.Process{ServiceName: "batch-process"}, Spans: []*model.Span{{OperationName: "test-op"}}},
			expected: []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "batch-process"}}},
		},
	}
	for _, test := range tests {
		_, err := client.PostSpans(context.Background(), &api_v2.PostSpansRequest{
			Batch: test.batch,
		})
		require.NoError(t, err)
		got := processor.getSpans()
		require.Equal(t, len(test.batch.GetSpans()), len(got))
		assert.Equal(t, test.expected, got)
		processor.reset()
	}
}

func TestGRPCCompressionEnabled(t *testing.T) {
	processor := &mockSpanProcessor{}
	server, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		handler := NewGRPCHandler(zap.NewNop(), processor, &tenancy.Manager{})
		api_v2.RegisterCollectorServiceServer(s, handler)
	})
	defer server.Stop()

	client, conn := newClient(t, addr)
	defer conn.Close()

	// Do not use string constant imported from grpc, since we are actually testing that package is imported by the handler.
	_, err := client.PostSpans(
		context.Background(),
		&api_v2.PostSpansRequest{},
		grpc.UseCompressor("gzip"),
	)
	require.NoError(t, err)
}

func TestPostSpansWithError(t *testing.T) {
	testCases := []struct {
		processorError error
		expectedError  string
		expectedLog    string
	}{
		{
			processorError: errors.New("test-error"),
			expectedError:  "test-error",
			expectedLog:    "test-error",
		},
		{
			processorError: processor.ErrBusy,
			expectedError:  "server busy",
		},
	}
	for _, test := range testCases {
		t.Run(test.expectedError, func(t *testing.T) {
			processor := &mockSpanProcessor{expectedError: test.processorError}
			logger, logBuf := testutils.NewLogger()
			server, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
				handler := NewGRPCHandler(logger, processor, &tenancy.Manager{})
				api_v2.RegisterCollectorServiceServer(s, handler)
			})
			defer server.Stop()
			client, conn := newClient(t, addr)
			defer conn.Close()
			r, err := client.PostSpans(context.Background(), &api_v2.PostSpansRequest{
				Batch: model.Batch{
					Spans: []*model.Span{
						{
							OperationName: "fake-operation",
						},
					},
				},
			})
			require.Error(t, err)
			require.Nil(t, r)
			assert.Contains(t, err.Error(), test.expectedError)
			assert.Contains(t, logBuf.String(), test.expectedLog)
			assert.Len(t, processor.getSpans(), 1)
		})
	}
}

// withMetadata returns a Context with metadata for outbound (client) calls
func withMetadata(ctx context.Context, headerName, headerValue string, t *testing.T) context.Context {
	t.Helper()

	md := metadata.New(map[string]string{headerName: headerValue})
	return metadata.NewOutgoingContext(ctx, md)
}

func TestPostTenantedSpans(t *testing.T) {
	tenantHeader := "x-tenant"
	dummyTenant := "grpc-test-tenant"

	processor := &mockSpanProcessor{}
	server, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		handler := NewGRPCHandler(zap.NewNop(), processor,
			tenancy.NewManager(&tenancy.Options{
				Enabled: true,
				Header:  tenantHeader,
				Tenants: []string{dummyTenant},
			}))
		api_v2.RegisterCollectorServiceServer(s, handler)
	})
	defer server.Stop()
	client, conn := newClient(t, addr)
	defer conn.Close()

	ctxWithTenant := withMetadata(context.Background(), tenantHeader, dummyTenant, t)
	ctxNoTenant := context.Background()
	mdTwoTenants := metadata.Pairs()
	mdTwoTenants.Set(tenantHeader, "a", "b")
	ctxTwoTenants := metadata.NewOutgoingContext(context.Background(), mdTwoTenants)
	ctxBadTenant := withMetadata(context.Background(), tenantHeader, "invalid-tenant", t)

	withMetadata(context.Background(),
		tenantHeader, dummyTenant, t)

	tests := []struct {
		name            string
		ctx             context.Context
		batch           model.Batch
		mustFail        bool
		expected        []*model.Span
		expectedTenants map[string]bool
	}{
		{
			name:  "valid tenant",
			ctx:   ctxWithTenant,
			batch: model.Batch{Process: &model.Process{ServiceName: "batch-process"}, Spans: []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "bar"}}}},

			mustFail:        false,
			expected:        []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "bar"}}},
			expectedTenants: map[string]bool{dummyTenant: true},
		},
		{
			name:  "no tenant",
			ctx:   ctxNoTenant,
			batch: model.Batch{Process: &model.Process{ServiceName: "batch-process"}, Spans: []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "bar"}}}},

			// Because NewGRPCHandler expects a tenant header, it will reject spans without one
			mustFail:        true,
			expected:        nil,
			expectedTenants: nil,
		},
		{
			name:  "two tenants",
			ctx:   ctxTwoTenants,
			batch: model.Batch{Process: &model.Process{ServiceName: "batch-process"}, Spans: []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "bar"}}}},

			// NewGRPCHandler rejects spans with multiple values for tenant header
			mustFail:        true,
			expected:        nil,
			expectedTenants: nil,
		},
		{
			name:  "invalid tenant",
			ctx:   ctxBadTenant,
			batch: model.Batch{Process: &model.Process{ServiceName: "batch-process"}, Spans: []*model.Span{{OperationName: "test-op", Process: &model.Process{ServiceName: "bar"}}}},

			// NewGRPCHandler rejects spans with multiple values for tenant header
			mustFail:        true,
			expected:        nil,
			expectedTenants: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.PostSpans(test.ctx, &api_v2.PostSpansRequest{
				Batch: test.batch,
			})
			if test.mustFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.expected, processor.getSpans())
			assert.Equal(t, test.expectedTenants, processor.getTenants())
			processor.reset()
		})
	}
}

// withIncomingMetadata returns a Context with metadata for a server to receive
func withIncomingMetadata(ctx context.Context, headerName, headerValue string, t *testing.T) context.Context {
	t.Helper()

	md := metadata.New(map[string]string{headerName: headerValue})
	return metadata.NewIncomingContext(ctx, md)
}

func TestGetTenant(t *testing.T) {
	tenantHeader := "some-tenant-header"
	validTenants := []string{"acme", "another-example"}

	mdTwoTenants := metadata.Pairs()
	mdTwoTenants.Set(tenantHeader, "a", "b")
	ctxTwoTenants := metadata.NewOutgoingContext(context.Background(), mdTwoTenants)

	tests := []struct {
		name     string
		ctx      context.Context
		tenant   string
		mustFail bool
	}{
		{
			name:     "valid tenant",
			ctx:      withIncomingMetadata(context.TODO(), tenantHeader, "acme", t),
			mustFail: false,
			tenant:   "acme",
		},
		{
			name:     "no tenant",
			ctx:      context.TODO(),
			mustFail: true,
			tenant:   "",
		},
		{
			name:     "two tenants",
			ctx:      ctxTwoTenants,
			mustFail: true,
			tenant:   "",
		},
		{
			name:     "invalid tenant",
			ctx:      withIncomingMetadata(context.TODO(), tenantHeader, "an-invalid-tenant", t),
			mustFail: true,
			tenant:   "",
		},
	}

	processor := &mockSpanProcessor{}
	handler := NewGRPCHandler(zap.NewNop(), processor,
		tenancy.NewManager(&tenancy.Options{
			Enabled: true,
			Header:  tenantHeader,
			Tenants: validTenants,
		}))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tenant, err := handler.batchConsumer.validateTenant(test.ctx)
			if test.mustFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.tenant, tenant)
		})
	}
}
