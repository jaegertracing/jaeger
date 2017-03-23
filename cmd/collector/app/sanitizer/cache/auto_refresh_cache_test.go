// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cache

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/cmd/collector/app/sanitizer/cache/mocks"
	"github.com/uber/jaeger/pkg/testutils"
)

var (
	testCache1 = map[string]string{"supply": "rt-supply"}
	testCache2 = map[string]string{"demand": "rt-demand"}
	errDefault = errors.New("Test error")
)

// Generate the serviceAliasMapping mocks using go generate. Run "make build-mocks" to regenerate mocks
//go:generate mockery -name=ServiceAliasMappingStorage
//go:generate mockery -name=ServiceAliasMappingExternalSource

func getCache(t *testing.T) (*autoRefreshCache, *mocks.ServiceAliasMappingExternalSource, *mocks.ServiceAliasMappingStorage) {
	mockExtSource := &mocks.ServiceAliasMappingExternalSource{}
	mockStorage := &mocks.ServiceAliasMappingStorage{}
	logger, _ := testutils.NewLogger(false)

	return &autoRefreshCache{
		cache:               make(map[string]string, 0),
		extSource:           mockExtSource,
		storage:             mockStorage,
		logger:              logger,
		readRefreshInterval: 5 * time.Millisecond,
		saveRefreshInterval: 5 * time.Millisecond,
		stopSaveChan:        make(chan struct{}),
		stopRefreshChan:     make(chan struct{}),
	}, mockExtSource, mockStorage
}

// sleepHelper Sleep until breakCondition is met
func sleepHelper(breakCondition func() bool, sleep time.Duration) {
	for i := 0; i < 100; i++ {
		if breakCondition() {
			break
		}
		time.Sleep(sleep)
	}
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

func TestRefreshFromStorageError(t *testing.T) {
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
	mS.On("Load").Return(testCache1, nil).Times(2)
	mE.On("Load").Return(testCache1, nil).Times(2)
	mS.On("Save", testCache1).Return(nil).Times(2)

	assert.Equal(t, "rt-demand", c.Get("demand"))

	c.initializeCacheRefresh()
	defer c.StopRefresh()
	sleepHelper(func() bool { return c.Get("demand") == "" }, 1*time.Millisecond)

	assert.Empty(t, c.Get("demand"), "The old cache should've been swapped out")
	assert.Equal(t, "rt-supply", c.Get("supply"))
}

func TestIsEmpty(t *testing.T) {
	c, _, _ := getCache(t)
	assert.True(t, c.IsEmpty())
	c.cache = testCache2
	assert.False(t, c.IsEmpty())
}
