// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagereceiver

import (
	"context"
	"errors"
	"log"
	"net"
	"testing"
	"time"

	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/model"
	grpcCfg "github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

type storageHost struct {
	t                *testing.T
	storageExtension component.Component
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	myMap := make(map[component.ID]component.Component)
	myMap[jaegerstorage.ID] = host.storageExtension
	return myMap
}

func (host storageHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	return nil
}

func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

var (
	services = []string{"example-service-1", "example-service-2"}
	spans    = []*model.Span{
		{
			TraceID: model.NewTraceID(0, 1),
			SpanID:  model.NewSpanID(1),
			Process: &model.Process{
				ServiceName: services[0],
			},
		},
		{
			TraceID: model.NewTraceID(0, 1),
			SpanID:  model.NewSpanID(2),
			Process: &model.Process{
				ServiceName: services[0],
			},
		},
		{
			TraceID: model.NewTraceID(0, 2),
			SpanID:  model.NewSpanID(3),
			Process: &model.Process{
				ServiceName: services[1],
			},
		},
		{
			TraceID: model.NewTraceID(0, 2),
			SpanID:  model.NewSpanID(4),
			Process: &model.Process{
				ServiceName: services[1],
			},
		},
	}
)

func TestReceiverNoStorageError(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.TraceStorage = "foo"

	r, err := newTracesReceiver(
		cfg,
		receivertest.NewNopCreateSettings(),
		consumertest.NewNop(),
	)
	require.NoError(t, err)

	err = r.Start(context.Background(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "cannot find storage factory")
}

func TestReceiverStart(t *testing.T) {
	ctx := context.Background()
	host := newStorageHost(t, "external-storage")

	cfg := createDefaultConfig().(*Config)
	cfg.TraceStorage = "external-storage"

	r, err := newTracesReceiver(
		cfg,
		receivertest.NewNopCreateSettings(),
		consumertest.NewNop(),
	)
	require.NoError(t, err)

	require.NoError(t, r.Start(ctx, host))
	require.NoError(t, r.Shutdown(ctx))
}

func TestReceiverStartConsume(t *testing.T) {
	sink := &consumertest.TracesSink{}

	cfg := createDefaultConfig().(*Config)
	cfg.TraceStorage = "external-storage"

	r, _ := newTracesReceiver(cfg, receivertest.NewNopCreateSettings(), sink)
	ctx, cancelFunc := context.WithCancel(context.Background())
	r.cancelConsumeLoop = cancelFunc

	tests := []struct {
		name           string
		services       []string
		traces         []*model.Trace
		tracesErr      error
		expectedTraces []*model.Trace
	}{
		{
			name: "empty service",
		},
		{
			name:      "find traces error",
			services:  []string{"example-service"},
			tracesErr: errors.New("failed to find traces"),
		},
		{
			name:     "consume first trace",
			services: []string{services[0]},
			traces: []*model.Trace{
				{Spans: []*model.Span{spans[0]}},
			},
			expectedTraces: []*model.Trace{
				{Spans: []*model.Span{spans[0]}},
			},
		},
		{
			name:     "consume second trace",
			services: services,
			traces: []*model.Trace{
				{Spans: []*model.Span{spans[0]}},
				{Spans: []*model.Span{spans[2], spans[3]}},
			},
			expectedTraces: []*model.Trace{
				{Spans: []*model.Span{spans[0]}},
				{Spans: []*model.Span{spans[2]}},
				{Spans: []*model.Span{spans[3]}},
			},
		},
		{
			name:     "re-consume first trace with new spans",
			services: services,
			traces: []*model.Trace{
				{Spans: []*model.Span{spans[0], spans[1]}},
				{Spans: []*model.Span{spans[2], spans[3]}},
			},
			expectedTraces: []*model.Trace{
				{Spans: []*model.Span{spans[0]}},
				{Spans: []*model.Span{spans[2]}},
				{Spans: []*model.Span{spans[3]}},
				// span at index 1 is consumed last
				{Spans: []*model.Span{spans[1]}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := new(mocks.Reader)
			reader.On("GetServices", mock.AnythingOfType("*context.cancelCtx")).Return(test.services, nil)
			for _, service := range test.services {
				reader.On(
					"FindTraces",
					mock.AnythingOfType("*context.cancelCtx"),
					&spanstore.TraceQueryParameters{ServiceName: service},
				).Return(test.traces, test.tracesErr)
			}
			r.spanReader = reader

			require.NoError(t, r.Shutdown(ctx))
			err := r.consumeLoop(ctx)
			require.EqualError(t, err, context.Canceled.Error())

			expectedTraces := make([]ptrace.Traces, 0)
			for _, trace := range test.expectedTraces {
				td, err := jaeger2otlp.ProtoToTraces([]*model.Batch{
					{
						Spans:   []*model.Span{trace.Spans[0]},
						Process: trace.Spans[0].Process,
					},
				})
				require.NoError(t, err)
				expectedTraces = append(expectedTraces, td)
			}
			actualTraces := sink.AllTraces()
			assert.Equal(t, expectedTraces, actualTraces)
		})
	}
}

func newStorageHost(t *testing.T, traceStorage string) *storageHost {
	ctx := context.Background()

	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")

	s := grpc.NewServer()
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
	t.Cleanup(s.Stop)

	f := jaegerstorage.NewFactory()
	cfg := &jaegerstorage.Config{
		GRPC: map[string]grpcCfg.Configuration{
			traceStorage: {
				RemoteServerAddr:     lis.Addr().String(),
				RemoteConnectTimeout: 1 * time.Second,
			},
		},
	}
	set := extension.CreateSettings{
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		BuildInfo:         component.NewDefaultBuildInfo(),
	}
	ext, err := f.CreateExtension(ctx, set, cfg)
	require.NoError(t, err)

	host := &storageHost{
		t:                t,
		storageExtension: ext,
	}

	err = host.storageExtension.Start(ctx, host)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, ext.Shutdown(ctx))
	})
	return host
}
