// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagecleaner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	factoryMocks "github.com/jaegertracing/jaeger/internal/storage/v1/mocks"
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
	name           string
	factory        storage.Factory
	metricsFactory storage.MetricStoreFactory
}

func (*mockStorageExt) Start(context.Context, component.Host) error {
	panic("not implemented")
}

func (*mockStorageExt) Shutdown(context.Context) error {
	panic("not implemented")
}

func (m *mockStorageExt) TraceStorageFactory(name string) (storage.Factory, bool) {
	if m.name == name {
		return m.factory, true
	}
	return nil, false
}

func (m *mockStorageExt) MetricStorageFactory(name string) (storage.MetricStoreFactory, bool) {
	if m.name == name {
		return m.metricsFactory, true
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
			factory: &PurgerFactory{err: errors.New("error")},
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
			s := newStorageCleaner(config, component.TelemetrySettings{
				Logger: zaptest.NewLogger(t),
			})
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
	s := newStorageCleaner(config, component.TelemetrySettings{
		Logger: zaptest.NewLogger(t),
	})
	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, &mockStorageExt{
		name:    "storage",
		factory: nil,
	})
	err := s.Start(context.Background(), host)
	require.ErrorContains(t, err, "cannot find storage factory")
}

func TestStorageExtensionStartError(t *testing.T) {
	config := &Config{
		TraceStorage: "storage",
		Port:         "invalid-port",
	}
	var startStatus atomic.Pointer[componentstatus.Event]
	s := newStorageCleaner(config, component.TelemetrySettings{
		Logger: zaptest.NewLogger(t),
	})
	host := &testHost{
		Host: storagetest.NewStorageHost().WithExtension(
			jaegerstorage.ID,
			&mockStorageExt{
				name:    "storage",
				factory: &PurgerFactory{},
			}),
		reportFunc: func(status *componentstatus.Event) {
			startStatus.Store(status)
		},
	}

	require.NoError(t, s.Start(context.Background(), host))
	assert.Eventually(t, func() bool {
		return startStatus.Load() != nil
	}, 5*time.Second, 100*time.Millisecond)
	require.ErrorContains(t, startStatus.Load().Err(), "error starting cleaner server")
}

type testHost struct {
	component.Host
	reportFunc func(event *componentstatus.Event)
}

func (h *testHost) Report(event *componentstatus.Event) {
	h.reportFunc(event)
}
