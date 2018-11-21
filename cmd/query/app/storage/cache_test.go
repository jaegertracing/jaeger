// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var _ io.Closer = &Cache{} // API compliance check

func initializeCache() (*Cache, *spanstoremocks.Reader) {
	mock := &spanstoremocks.Reader{}
	return NewCache(mock, 50*time.Millisecond, zap.NewNop()), mock
}

func TestCacheInitialize(t *testing.T) {
	goodCache, goodMock := initializeCache()
	expectedServices := []string{"someService"}
	goodMock.On("GetServices").Return(expectedServices, nil)
	goodCache.Initialize()
	goodMock.AssertCalled(t, "GetServices")

	badCache, badMock := initializeCache()
	err := fmt.Errorf("Why u no work")
	badMock.On("GetServices").Return(nil, err)
	badCache.Initialize()
	badMock.AssertCalled(t, "GetServices")
}

func TestCacheQuery(t *testing.T) {
	cache, mock := initializeCache()

	expectedTraces := []*model.Trace{}
	mock.On("FindTraces", &spanstore.TraceQueryParameters{}).Return(expectedTraces, nil).Times(1)

	actualTraces, err := cache.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
	assert.NoError(t, err)
	assert.Equal(t, expectedTraces, actualTraces)
}

func TestGetTrace(t *testing.T) {
	cache, mock := initializeCache()

	expectedTraces := &model.Trace{}
	traceID := model.TraceID{
		High: 1,
		Low:  2,
	}
	mock.On("GetTrace", traceID).Return(expectedTraces, nil).Times(1)

	actualTraces, err := cache.GetTrace(context.Background(), traceID)
	assert.NoError(t, err)
	assert.Equal(t, expectedTraces, actualTraces)
}

func TestGetOperations(t *testing.T) {
	cache, mock := initializeCache()

	expectedOperations := []string{"get", "post"}
	service := "service"
	mock.On("GetOperations", service).Return(expectedOperations, nil).Times(1)

	actualTraces, err := cache.GetOperations(context.Background(), service)
	assert.NoError(t, err)
	assert.Equal(t, expectedOperations, actualTraces)
}

func TestWarmCache(t *testing.T) {
	cache, mock := initializeCache()

	expectedServices := []string{"someService"}
	mock.On("GetServices").Return(expectedServices, nil).Times(1)

	err := cache.cache.warmCache()
	assert.NoError(t, err)

	services, err := cache.GetServices(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, expectedServices, services)
}

func TestRefreshFromStorage(t *testing.T) {
	cache, mock := initializeCache()

	expectedServices := []string{"someService"}
	mock.On("GetServices").Return(expectedServices, nil).Times(1)

	cache.cache.initializeCacheRefresh()

	for i := 0; i < 100; i++ {
		if !cache.cache.isEmpty() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	services, err := cache.GetServices(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, expectedServices, services)

	cache.Close()
}
