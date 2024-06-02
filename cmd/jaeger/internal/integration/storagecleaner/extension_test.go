// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagecleaner

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
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

func (f *PurgerFactory) Purge(_ context.Context) error {
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
		{
			name:    "good storage",
			factory: &PurgerFactory{},
			status:  http.StatusOK,
		},
		{
			name:    "good storage with error",
			factory: &PurgerFactory{err: fmt.Errorf("error")},
			status:  http.StatusInternalServerError,
		},
		{
			name:    "bad storage",
			factory: &factoryMocks.Factory{},
			status:  http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := &Config{
				TraceStorage: "storage",
				Port:         Port,
			}
			s := newStorageCleaner(config, component.TelemetrySettings{})
			require.NotEmpty(t, s.Dependencies())
			host := storagetest.NewStorageHost()
			host.WithExtension(jaegerstorage.ID, &mockStorageExt{
				name:    "storage",
				factory: test.factory,
			})
			require.NoError(t, s.Start(context.Background(), host))
			defer s.Shutdown(context.Background())

			addr := fmt.Sprintf("http://0.0.0.0:%s%s", Port, URL)
			client := &http.Client{}
			require.Eventually(t, func() bool {
				r, err := http.NewRequest(http.MethodPost, addr, nil)
				require.NoError(t, err)
				resp, err := client.Do(r)
				require.NoError(t, err)
				defer resp.Body.Close()
				return test.status == resp.StatusCode
			}, 5*time.Second, 100*time.Millisecond)
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

func TestStorageExtensionStartError(t *testing.T) {
	config := &Config{
		TraceStorage: "storage",
		Port:         "invalid-port",
	}
	var startStatus atomic.Pointer[component.StatusEvent]
	s := newStorageCleaner(config, component.TelemetrySettings{
		ReportStatus: func(status *component.StatusEvent) {
			startStatus.Store(status)
		},
	})
	host := storagetest.NewStorageHost().WithExtension(
		jaegerstorage.ID,
		&mockStorageExt{
			name:    "storage",
			factory: &PurgerFactory{},
		})
	require.NoError(t, s.Start(context.Background(), host))
	assert.Eventually(t, func() bool {
		return startStatus.Load() != nil
	}, 5*time.Second, 100*time.Millisecond)
	require.Contains(t, startStatus.Load().Err().Error(), "error starting cleaner server")
}
