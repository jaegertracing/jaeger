// Copyright (c) 2018 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package cache

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/cache/mocks"
)

var (
	testCache1 = map[string]string{"supply": "rt-supply"}
	testCache2 = map[string]string{"demand": "rt-demand"}
	errDefault = errors.New("Test error")
)

// Generate the serviceAliasMapping mocks using go generate. Run "make build-mocks" to regenerate mocks
// dont_go:generate mockery -name=ServiceAliasMappingStorage
// dont_go:generate mockery -name=ServiceAliasMappingExternalSource

func getCache(t *testing.T) (*autoRefreshCache, *mocks.ServiceAliasMappingExternalSource, *mocks.ServiceAliasMappingStorage) {
	mockExtSource := &mocks.ServiceAliasMappingExternalSource{}
	mockStorage := &mocks.ServiceAliasMappingStorage{}
	logger := zap.NewNop()

	return &autoRefreshCache{
		cache:               make(map[string]string),
		extSource:           mockExtSource,
		storage:             mockStorage,
		logger:              logger,
		readRefreshInterval: time.Millisecond,
		saveRefreshInterval: time.Millisecond,
		stopSaveChan:        make(chan struct{}),
		stopRefreshChan:     make(chan struct{}),
	}, mockExtSource, mockStorage
}

// sleepHelper sleeps until breakCondition is met
func sleepHelper(breakCondition func() bool, sleep time.Duration) {
	for i := 0; i < 100; i++ {
		if breakCondition() {
			break
		}
		time.Sleep(sleep)
	}
}

func TestConstructor(t *testing.T) {
	c := NewAutoRefreshCache(nil, nil, zap.NewNop(), 0, 0)
	assert.NotNil(t, c)
}

func TestGetRandomSleepTime(t *testing.T) {
	interval := getRandomSleepTime(5 * time.Minute)
	assert.True(t, 0 <= interval && interval <= 5*time.Minute)
}

func TestGet(t *testing.T) {
	c, _, _ := getCache(t)
	c.cache["rt-supply"] = "rt-supply"
	v := c.Get("rt-supply")
	assert.Equal(t, "rt-supply", v)

	assert.Empty(t, c.Get("fail"), "Getting non-existing value")
}

func TestPut(t *testing.T) {
	c, _, _ := getCache(t)
	c.Put("key", "value")
	assert.Empty(t, c.Get("key"))
}

func TestInitialize(t *testing.T) {
	c, mE, mS := getCache(t)
	defer c.StopRefresh()

	mS.On("Load").Return(testCache1, nil)
	mE.On("Load").Return(nil, errDefault)
	assert.NoError(t, c.Initialize())
	assert.Equal(t, "rt-supply", c.Get("supply"))
}

func TestInitialize_error(t *testing.T) {
	c, mE, mS := getCache(t)
	defer c.StopRefresh()

	mS.On("Load").Return(nil, errDefault)
	mE.On("Load").Return(nil, errDefault)
	assert.NoError(t, c.Initialize())
	assert.True(t, c.IsEmpty())
}

func TestWarmCache(t *testing.T) {
	c, mE, mS := getCache(t)
	mS.On("Load").Return(testCache1, nil).Times(1)
	err := c.warmCache()
	assert.NoError(t, err)
	assert.Equal(t, "rt-supply", c.Get("supply"))

	mS.On("Load").Return(nil, errDefault).Times(1)
	mE.On("Load").Return(testCache2, nil).Times(1)
	err = c.warmCache()
	assert.NoError(t, err)
	assert.Equal(t, "rt-demand", c.Get("demand"), "External load should've succeeded")

	mS.On("Load").Return(nil, errDefault).Times(1)
	mE.On("Load").Return(nil, errDefault).Times(1)
	assert.Error(t, c.warmCache(), "Both loads should've failed")
}

func TestRefreshFromStorage(t *testing.T) {
	c, _, mS := getCache(t)
	mS.On("Load").Return(testCache1, nil)

	go c.refreshFromStorage(5 * time.Millisecond)
	defer c.StopRefresh()
	sleepHelper(func() bool { return !c.IsEmpty() }, 1*time.Millisecond)
	assert.Equal(t, "rt-supply", c.Get("supply"))
}

func TestRefreshFromStorage_error(t *testing.T) {
	c, _, mS := getCache(t)
	mS.On("Load").Return(nil, errDefault)

	go c.refreshFromStorage(5 * time.Millisecond)
	defer c.StopRefresh()
	time.Sleep(10 * time.Millisecond)
	assert.Empty(t, c.Get("supply"), "Storage load should've failed")
}

func TestInitializeCacheRefresh(t *testing.T) {
	c, mE, mS := getCache(t)
	c.cache = testCache2
	mS.On("Load").Return(testCache1, nil)
	mE.On("Load").Return(testCache1, nil)
	mS.On("Save", testCache1).Return(nil)

	assert.Equal(t, "rt-demand", c.Get("demand"))

	c.initializeCacheRefresh()
	defer c.StopRefresh()
	sleepHelper(func() bool { return c.Get("demand") == "" }, time.Millisecond)

	assert.Empty(t, c.Get("demand"), "The old cache should've been swapped out")
	assert.Equal(t, "rt-supply", c.Get("supply"))
}

func TestRefreshFromExternalSource(t *testing.T) {
	c, mE, mS := getCache(t)
	c.cache = testCache2
	mE.On("Load").Return(testCache1, nil)
	mS.On("Save", testCache1).Return(nil)

	assert.Equal(t, "rt-demand", c.Get("demand"))

	go c.refreshFromExternalSource(time.Millisecond)

	// TODO (wjang) there isn't really a better way to test refreshFromExternalSource
	time.Sleep(10 * time.Millisecond)
	c.StopRefresh()

	mE.AssertCalled(t, "Load")
	mS.AssertCalled(t, "Save", testCache1)
}

func TestUpdateAndSaveToStorage(t *testing.T) {
	tests := []struct {
		initialCache      map[string]string
		loadCache         map[string]string
		loadErr           error
		saveNumberOfCalls int
		caption           string
	}{
		{
			initialCache: testCache1,
			loadErr:      errDefault,
			caption:      "load error",
		},
		{
			initialCache: testCache1,
			loadCache:    testCache1,
			caption:      "same cache",
		},
		{
			initialCache:      testCache1,
			loadCache:         testCache2,
			saveNumberOfCalls: 1,
			caption:           "different cache",
		},
	}

	for _, test := range tests {
		tt := test // capture var
		t.Run(tt.caption, func(t *testing.T) {
			c, mE, mS := getCache(t)
			c.cache = tt.initialCache

			mE.On("Load").Return(tt.loadCache, tt.loadErr)
			mS.On("Save", tt.loadCache).Return(nil)
			c.updateAndSaveToStorage()
			mS.AssertNumberOfCalls(t, "Save", tt.saveNumberOfCalls)
		})
	}
}

func TestIsEmpty(t *testing.T) {
	c, _, _ := getCache(t)
	assert.True(t, c.IsEmpty())
	c.cache = testCache2
	assert.False(t, c.IsEmpty())
}
