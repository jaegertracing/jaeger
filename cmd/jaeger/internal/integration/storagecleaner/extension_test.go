// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagecleaner

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/storage"
	factoryMocks "github.com/jaegertracing/jaeger/storage/mocks"
)

var (
	_ jaegerstorage.Extension = (*mockStorageExt)(nil)
	_ storage.Factory         = (*PurgerFactory)(nil)
)

type PurgerFactory struct {
	factoryMocks.Factory
	err error
}

func (f *PurgerFactory) Purge() error {
	return f.err
}

type mockStorageExt struct {
	name    string
	factory storage.Factory
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
	tests := []struct {
		name    string
		factory storage.Factory
		status  int
	}{
		{"good storage", &PurgerFactory{}, http.StatusOK},
		{"good storage with error", &PurgerFactory{err: fmt.Errorf("error")}, http.StatusInternalServerError},
		{"bad storage", &factoryMocks.Factory{}, http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := &Config{
				TraceStorage: "storage",
				Port:         Port,
			}
			s := newStorageCleaner(config, component.TelemetrySettings{})
			host := storagetest.NewStorageHost()
			host.WithExtension(jaegerstorage.ID, &mockStorageExt{
				name:    "storage",
				factory: test.factory,
			})
			ctx := context.Background()
			err := s.Start(ctx, host)
			defer s.Shutdown(ctx)
			require.NoError(t, err)

			addr := fmt.Sprintf("http://0.0.0.0:%s%s", Port, URL)
			client := &http.Client{}
			var resp *http.Response
			require.Eventually(t, func() bool {
				r, err := http.NewRequest(http.MethodPost, addr, nil)
				require.NoError(t, err)
				resp, err = client.Do(r)
				require.NoError(t, err)
				defer resp.Body.Close()
				return err == nil
			}, 5*time.Second, 100*time.Millisecond)
			require.Equal(t, test.status, resp.StatusCode)
			s.Dependencies()
		})
	}
}

func TestGetStorageFactoryError(t *testing.T) {
	config := &Config{}
	s := newStorageCleaner(config, component.TelemetrySettings{})
	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, &mockStorageExt{
		name:    "storage",
		factory: nil,
	})
	err := s.Start(context.Background(), host)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot find storage factory")
}
