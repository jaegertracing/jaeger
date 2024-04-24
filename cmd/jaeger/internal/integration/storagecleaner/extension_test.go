package storagecleaner

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/storage"
	factoryMocks "github.com/jaegertracing/jaeger/storage/mocks"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
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

func TestStorageCleanerExtension(t *testing.T) {
	config := &Config{
		TraceStorage: "storage",
		Port:         Port,
	}
	s := newStorageCleaner(config)
	host := storagetest.NewStorageHost()
	factory := &factoryMocks.Factory{}
	host.WithExtension(jaegerstorage.ID, &mockStorageExt{
		name:    "storage",
		factory: factory,
	})
	err := s.Start(context.Background(), host)
	require.NoError(t, err)

	Addr := fmt.Sprintf("http://0.0.0.0:%s%s", Port, URL)
	client := &http.Client{}
	var ok bool
	var resp *http.Response
	for i := 0; i < 10; i++ {
		r, err := http.NewRequest(http.MethodPost, Addr, nil)
		require.NoError(t, err)

		resp, err = client.Do(r)
		if err == nil {
			ok = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.True(t, ok, "could not reach storage cleaner server")
	defer resp.Body.Close()
	require.Equal(t, resp.StatusCode, http.StatusOK)
	s.Dependencies()
	s.Shutdown(context.Background())
}

func TestStorageGetStorageFactoryError(t *testing.T) {
	config := &Config{}
	s := newStorageCleaner(config)
	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, &mockStorageExt{
		name:    "storage",
		factory: nil,
	})
	err := s.Start(context.Background(), host)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot find storage factory")

}
