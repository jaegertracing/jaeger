// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagereceiver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage"
	factoryMocks "github.com/jaegertracing/jaeger/storage/mocks"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var _ jaegerstorage.Extension = (*mockStorageExt)(nil)

type mockStorageExt struct {
	name    string
	factory *factoryMocks.Factory
}

func (m *mockStorageExt) Start(ctx context.Context, host component.Host) error {
	panic("not implemented")
}

func (m *mockStorageExt) Shutdown(ctx context.Context) error {
	panic("not implemented")
}

func (m *mockStorageExt) Factory(name string) (storage.Factory, bool) {
	if m.name == name {
		return m.factory, true
	}
	return nil, false
}

type receiverTest struct {
	storageName     string
	receiveName     string
	receiveInterval time.Duration
	reportStatus    func(*component.StatusEvent)

	reader   *spanStoreMocks.Reader
	factory  *factoryMocks.Factory
	host     *storagetest.StorageHost
	receiver *storageReceiver
}

func withReceiver(
	r *receiverTest,
	fn func(r *receiverTest),
) {
	reader := new(spanStoreMocks.Reader)
	factory := new(factoryMocks.Factory)
	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, &mockStorageExt{
		name:    r.storageName,
		factory: factory,
	})
	cfg := createDefaultConfig().(*Config)
	cfg.TraceStorage = r.receiveName
	cfg.PullInterval = r.receiveInterval
	receiver, _ := newTracesReceiver(
		cfg,
		receivertest.NewNopCreateSettings(),
		consumertest.NewNop(),
	)
	receiver.settings.ReportStatus = func(se *component.StatusEvent) {}

	r.reader = reader
	r.factory = factory
	r.host = host
	r.receiver = receiver
	fn(r)
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

func TestReceiver_NoStorageError(t *testing.T) {
	r := &receiverTest{
		storageName: "",
		receiveName: "foo",
	}
	withReceiver(r, func(r *receiverTest) {
		err := r.receiver.Start(context.Background(), r.host)
		require.ErrorContains(t, err, "cannot find storage factory")
	})
}

func TestReceiver_CreateSpanReaderError(t *testing.T) {
	r := &receiverTest{
		storageName: "foo",
		receiveName: "foo",
	}
	withReceiver(r, func(r *receiverTest) {
		r.factory.On("CreateSpanReader").Return(nil, errors.New("mocked error"))

		err := r.receiver.Start(context.Background(), r.host)
		require.ErrorContains(t, err, "cannot create span reader")
	})
}

func TestReceiver_GetServiceError(t *testing.T) {
	r := &receiverTest{
		storageName: "external-storage",
		receiveName: "external-storage",
	}
	withReceiver(r, func(r *receiverTest) {
		r.reader.On("GetServices", mock.AnythingOfType("*context.cancelCtx")).Return([]string{}, errors.New("mocked error"))
		r.factory.On("CreateSpanReader").Return(r.reader, nil)
		r.receiver.spanReader = r.reader
		r.reportStatus = func(se *component.StatusEvent) {
			require.ErrorContains(t, se.Err(), "mocked error")
		}

		require.NoError(t, r.receiver.Start(context.Background(), r.host))
	})
}

func TestReceiver_Shutdown(t *testing.T) {
	withReceiver(&receiverTest{}, func(r *receiverTest) {
		require.NoError(t, r.receiver.Shutdown(context.Background()))
	})
}

func TestReceiver_Start(t *testing.T) {
	r := &receiverTest{
		storageName:     "external-storage",
		receiveName:     "external-storage",
		receiveInterval: 50 * time.Millisecond,
	}
	withReceiver(r, func(r *receiverTest) {
		r.reader.On("GetServices", mock.AnythingOfType("*context.cancelCtx")).Return([]string{}, nil)
		r.factory.On("CreateSpanReader").Return(r.reader, nil)

		require.NoError(t, r.receiver.Start(context.Background(), r.host))
		// let the consumeLoop to reach the end of iteration and sleep
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, r.receiver.Shutdown(context.Background()))
	})
}

func TestReceiver_StartConsume(t *testing.T) {
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

	withReceiver(&receiverTest{}, func(r *receiverTest) {
		sink := &consumertest.TracesSink{}
		r.receiver.nextConsumer = sink

		ctx, cancelFunc := context.WithCancel(context.Background())
		r.receiver.cancelConsumeLoop = cancelFunc

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				reader := new(spanStoreMocks.Reader)
				reader.On("GetServices", mock.AnythingOfType("*context.cancelCtx")).Return(test.services, nil)
				reader.On(
					"FindTraces",
					mock.AnythingOfType("*context.cancelCtx"),
					mock.AnythingOfType("*spanstore.TraceQueryParameters"),
				).Return(test.traces, test.tracesErr)
				r.receiver.spanReader = reader

				require.NoError(t, r.receiver.Shutdown(ctx))
				require.NoError(t, r.receiver.consumeLoop(ctx))

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
	})
}
