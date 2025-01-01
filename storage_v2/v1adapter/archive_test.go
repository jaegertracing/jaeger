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
	tests := []struct {
		name       string
		factory    storage.Factory
		wantReader spanstore.Reader
		wantWriter spanstore.Writer
	}{
		{
			name:       "Archive storage not supported by the factory",
			factory:    new(fakeStorageFactory1),
			wantReader: nil,
			wantWriter: nil,
		},
		{
			name:       "Archive storage not configured",
			factory:    &fakeStorageFactory2{rErr: storage.ErrArchiveStorageNotConfigured},
			wantReader: nil,
			wantWriter: nil,
		},
		{
			name:       "Archive storage not supported for reader",
			factory:    &fakeStorageFactory2{rErr: storage.ErrArchiveStorageNotSupported},
			wantReader: nil,
			wantWriter: nil,
		},
		{
			name:       "Error initializing archive span reader",
			factory:    &fakeStorageFactory2{rErr: assert.AnError},
			wantReader: nil,
			wantWriter: nil,
		},
		{
			name:       "Archive storage not supported for writer",
			factory:    &fakeStorageFactory2{wErr: storage.ErrArchiveStorageNotSupported},
			wantReader: nil,
			wantWriter: nil,
		},
		{
			name:       "Error initializing archive span writer",
			factory:    &fakeStorageFactory2{wErr: assert.AnError},
			wantReader: nil,
			wantWriter: nil,
		},
		{
			name:       "Successfully initialize archive storage",
			factory:    &fakeStorageFactory2{r: &spanstoremocks.Reader{}, w: &spanstoremocks.Writer{}},
			wantReader: &spanstoremocks.Reader{},
			wantWriter: &spanstoremocks.Writer{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := zap.NewNop()
			reader, writer := InitializeArchiveStorage(test.factory, logger)
			if test.wantReader != nil {
				require.Equal(t, test.wantReader, reader.spanReader)
			} else {
				require.Nil(t, reader)
			}
			if test.wantWriter != nil {
				require.Equal(t, test.wantWriter, writer.spanWriter)
			} else {
				require.Nil(t, writer)
			}
		})
	}
}
