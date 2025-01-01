package v1adapter

import (
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
	"go.uber.org/zap"
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
		assert.Nil(t, reader)
		assert.Nil(t, writer)
	})

	t.Run("Archive storage not configured", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{rErr: storage.ErrArchiveStorageNotConfigured},
			logger,
		)
		assert.Nil(t, reader)
		assert.Nil(t, writer)
	})

	t.Run("Archive storage not supported", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{rErr: storage.ErrArchiveStorageNotSupported},
			logger,
		)
		assert.Nil(t, reader)
		assert.Nil(t, writer)
	})

	t.Run("Error initializing archive span reader", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{rErr: assert.AnError},
			logger,
		)
		assert.Nil(t, reader)
		assert.Nil(t, writer)
	})

	t.Run("Error initializing archive span writer", func(t *testing.T) {
		reader, writer := InitializeArchiveStorage(
			&fakeStorageFactory2{wErr: assert.AnError},
			logger,
		)
		assert.Nil(t, reader)
		assert.Nil(t, writer)
	})

	t.Run("Successfully initialize archive storage", func(t *testing.T) {
		reader := &spanstoremocks.Reader{}
		writer := &spanstoremocks.Writer{}
		traceReader, traceWriter := InitializeArchiveStorage(
			&fakeStorageFactory2{r: reader, w: writer},
			logger,
		)
		assert.NotNil(t, traceReader)
		assert.NotNil(t, traceWriter)
		assert.Equal(t, reader, traceReader.spanReader)
		assert.Equal(t, writer, traceWriter.spanWriter)
	})
}
