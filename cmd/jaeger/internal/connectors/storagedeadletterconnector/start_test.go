// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagedeadletterconnector

import (
	"context"
	"errors"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector/connectortest"
	"go.opentelemetry.io/collector/consumer/consumertest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

// mockStorageExt is a minimal jaeger_storage extension serving one named trace-store
// factory, so Start's default resolveWriter (GetTraceStoreFactory + CreateTraceWriter)
// runs against a real host without a full backend.
type mockStorageExt struct {
	name    string
	factory tracestore.Factory
}

var _ jaegerstorage.Extension = (*mockStorageExt)(nil)

func (*mockStorageExt) Start(context.Context, component.Host) error { return nil }
func (*mockStorageExt) Shutdown(context.Context) error              { return nil }

func (m *mockStorageExt) TraceStorageFactory(name string) (tracestore.Factory, error) {
	if m.name == name {
		return m.factory, nil
	}
	return nil, errors.New("storage not found")
}

func (*mockStorageExt) MetricStorageFactory(string) (storage.MetricStoreFactory, error) {
	return nil, errors.New("metric storage not found")
}

func newFactoryConnector(t *testing.T, storeName string) *connectorImpl {
	set := connectortest.NewNopSettings(componentType)
	conn, err := createTracesToTraces(context.Background(), set, &Config{TraceStorage: storeName}, new(consumertest.TracesSink))
	require.NoError(t, err)
	return conn.(*connectorImpl)
}

func TestStart_ResolvesRealWriter(t *testing.T) {
	factory := new(tracestoremocks.Factory)
	writer := new(tracestoremocks.Writer)
	factory.On("CreateTraceWriter").Return(writer, nil)

	host := storagetest.NewStorageHost().WithExtension(
		jaegerstorage.ID, &mockStorageExt{name: "somestore", factory: factory},
	)

	c := newFactoryConnector(t, "somestore")
	require.NoError(t, c.Start(context.Background(), host))
	assert.Same(t, writer, c.traceWriter)
}

func TestStart_StorageFactoryNotFound(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(
		jaegerstorage.ID, &mockStorageExt{name: "othername"},
	)

	c := newFactoryConnector(t, "somestore")
	err := c.Start(context.Background(), host)
	require.ErrorContains(t, err, "cannot find storage factory")
}

func TestStart_CreateTraceWriterError(t *testing.T) {
	factory := new(tracestoremocks.Factory)
	factory.On("CreateTraceWriter").Return(nil, errors.New("boom"))

	host := storagetest.NewStorageHost().WithExtension(
		jaegerstorage.ID, &mockStorageExt{name: "somestore", factory: factory},
	)

	c := newFactoryConnector(t, "somestore")
	err := c.Start(context.Background(), host)
	require.ErrorContains(t, err, "cannot create trace writer")
}
