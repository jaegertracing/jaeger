// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

type fakeStorageFactory1 struct{}

type fakeStorageFactory2 struct {
	fakeStorageFactory1
	r    spanstore.Reader
	w    spanstore.Writer
	rErr error
	wErr error
}

func (*fakeStorageFactory1) Initialize(metrics.Factory, *zap.Logger) error {
	return nil
}
func (*fakeStorageFactory1) CreateSpanReader() (spanstore.Reader, error)             { return nil, nil }
func (*fakeStorageFactory1) CreateSpanWriter() (spanstore.Writer, error)             { return nil, nil }
func (*fakeStorageFactory1) CreateDependencyReader() (dependencystore.Reader, error) { return nil, nil }

func (f *fakeStorageFactory2) CreateArchiveSpanReader() (spanstore.Reader, error) { return f.r, f.rErr }
func (f *fakeStorageFactory2) CreateArchiveSpanWriter() (spanstore.Writer, error) { return f.w, f.wErr }

var (
	_ storage.Factory        = new(fakeStorageFactory1)
	_ storage.ArchiveFactory = new(fakeStorageFactory2)
)

func TestInitializeArchiveStorage(t *testing.T) {
	logger := zap.NewNop()

	t.Run("Archive storage not supported by the factory", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(new(fakeStorageFactory1), logger)
		require.Nil(t, reader)
		require.Nil(t, writer)
	})

	t.Run("Archive storage not configured", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{rErr: storage.ErrArchiveStorageNotConfigured},
			logger,
		)
		require.Nil(t, reader)
		require.Nil(t, writer)
	})

	t.Run("Archive storage not supported for reader", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{rErr: storage.ErrArchiveStorageNotSupported},
			logger,
		)
		require.Nil(t, reader)
		require.Nil(t, writer)
	})

	t.Run("Error initializing archive span reader", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{rErr: assert.AnError},
			logger,
		)
		require.Nil(t, reader)
		require.Nil(t, writer)
	})

	t.Run("Archive storage not supported for writer", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{wErr: storage.ErrArchiveStorageNotSupported},
			logger,
		)
		require.Nil(t, reader)
		require.Nil(t, writer)
	})

	t.Run("Error initializing archive span writer", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{wErr: assert.AnError},
			logger,
		)
		require.Nil(t, reader)
		require.Nil(t, writer)
	})

	t.Run("Successfully initialize archive storage", func(t *testing.T) {
		reader := &spanstoremocks.Reader{}
		writer := &spanstoremocks.Writer{}
		traceReader, traceWriter := InitializeArchiveStorage(
			&fakeStorageFactory2{r: reader, w: writer},
			logger,
		)
		require.Equal(t, reader, traceReader.spanReader)
		require.Equal(t, writer, traceWriter.spanWriter)
	})
}
