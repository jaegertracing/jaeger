// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var _ processor.SpanProcessor = (*mockSpanProcessor)(nil)

type mockSpanProcessor struct {
	expectedError error
	mux           sync.Mutex
	spans         []*model.Span
	traces        []ptrace.Traces
	tenants       map[string]bool
	transport     processor.InboundTransport
	spanFormat    processor.SpanFormat
}

func (p *mockSpanProcessor) ProcessSpans(_ context.Context, batch processor.Batch) ([]bool, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	batch.GetSpans(func(spans []*model.Span) {
		p.spans = append(p.spans, spans...)
	}, func(td ptrace.Traces) {
		p.traces = append(p.traces, td)
	})
	oks := make([]bool, len(p.spans))
	if p.tenants == nil {
		p.tenants = make(map[string]bool)
	}
	p.tenants[batch.GetTenant()] = true
	p.transport = batch.GetInboundTransport()
	p.spanFormat = batch.GetSpanFormat()
	return oks, p.expectedError
}

func (p *mockSpanProcessor) getSpans() []*model.Span {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.spans
}

func (p *mockSpanProcessor) getTraces() []ptrace.Traces {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.traces
}

func (p *mockSpanProcessor) getTenants() map[string]bool {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.tenants
}

func (p *mockSpanProcessor) getTransport() processor.InboundTransport {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.transport
}

func (p *mockSpanProcessor) getSpanFormat() processor.SpanFormat {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.spanFormat
}

func (p *mockSpanProcessor) reset() {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.spans = nil
	p.tenants = nil
	p.transport = ""
	p.spanFormat = ""
}

func (*mockSpanProcessor) Close() error {
	return nil
}

func initializeGRPCTestServer(t *testing.T, beforeServe func(s *grpc.Server)) (*grpc.Server, net.Addr) {
	server := grpc.NewServer()
	beforeServe(server)
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	go func() {
		err := server.Serve(lis)
		assert.NoError(t, err)
	}()
	return server, lis.Addr()
}

func newClient(t *testing.T, addr net.Addr) (api_v2.CollectorServiceClient, *grpc.ClientConn) {
	conn, err := grpc.NewClient(addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	return api_v2.NewCollectorServiceClient(conn), conn
}

func TestPostSpans(t *testing.T) {
	proc := &mockSpanProcessor{}
	server, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		handler := NewGRPCHandler(zap.NewNop(), proc, &tenancy.Manager{})
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
		got := proc.getSpans()
		require.Len(t, test.batch.GetSpans(), len(got))
		assert.Equal(t, test.expected, got)
		proc.reset()
	}
}

func TestGRPCCompressionEnabled(t *testing.T) {
	proc := &mockSpanProcessor{}
	server, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		handler := NewGRPCHandler(zap.NewNop(), proc, &tenancy.Manager{})
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
			require.ErrorContains(t, err, test.expectedError)
			require.Nil(t, r)
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

	proc := &mockSpanProcessor{}
	server, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		handler := NewGRPCHandler(zap.NewNop(), proc,
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
			assert.Equal(t, test.expected, proc.getSpans())
			assert.Equal(t, test.expectedTenants, proc.getTenants())
			proc.reset()
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

	proc := &mockSpanProcessor{}
	handler := NewGRPCHandler(zap.NewNop(), proc,
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

func TestBatchConsumer(t *testing.T) {
	tests := []struct {
		name               string
		batch              model.Batch
		transport          processor.InboundTransport
		spanFormat         processor.SpanFormat
		expectedTransport  processor.InboundTransport
		expectedSpanFormat processor.SpanFormat
	}{
		{
			name: "batchconsumer passes provided span options to processor",
			batch: model.Batch{
				Process: &model.Process{ServiceName: "testservice"},
				Spans: []*model.Span{
					{OperationName: "test-op", Process: &model.Process{ServiceName: "foo"}},
				},
			},
			transport:          processor.GRPCTransport,
			spanFormat:         processor.OTLPSpanFormat,
			expectedTransport:  processor.GRPCTransport,
			expectedSpanFormat: processor.OTLPSpanFormat,
		},
	}

	logger, _ := testutils.NewLogger()
	for _, tc := range tests {
		t.Parallel()
		t.Run(tc.name, func(t *testing.T) {
			processor := mockSpanProcessor{}
			batchConsumer := newBatchConsumer(logger, &processor, tc.transport, tc.spanFormat, tenancy.NewManager(&tenancy.Options{}))
			err := batchConsumer.consume(context.Background(), &model.Batch{
				Process: &model.Process{ServiceName: "testservice"},
				Spans: []*model.Span{
					{OperationName: "test-op", Process: &model.Process{ServiceName: "foo"}},
				},
			})
			require.NoError(t, err)
			assert.Equal(t, tc.transport, processor.getTransport())
			assert.Equal(t, tc.expectedSpanFormat, processor.getSpanFormat())
		})
	}
}
