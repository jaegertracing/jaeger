// Copyright (c) 2019 The Jaeger Authors.
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
package spanstore

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

/*
	Additional cache store tests that need to access internal parts. As such, package must be spanstore and not spanstore_test
*/

func TestExpiredItems(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		cache := NewCacheStore(store, time.Duration(-1*time.Hour), false)

		expireTime := uint64(time.Now().Add(cache.ttl).Unix())

		// Expired service

		cache.Update("service1", "op1", expireTime)
		cache.Update("service1", "op2", expireTime)

		services, err := cache.GetServices()
		assert.NoError(t, err)
		assert.Equal(t, 0, len(services)) // Everything should be expired

		// Expired service for operations

		cache.Update("service1", "op1", expireTime)
		cache.Update("service1", "op2", expireTime)

		operations, err := cache.GetOperations("service1")
		assert.NoError(t, err)
		assert.Equal(t, 0, len(operations)) // Everything should be expired

		// Expired operations, stable service

		cache.Update("service1", "op1", expireTime)
		cache.Update("service1", "op2", expireTime)

		cache.services["service1"] = uint64(time.Now().Unix() + 1e10)

		operations, err = cache.GetOperations("service1")
		assert.NoError(t, err)
		assert.Equal(t, 0, len(operations)) // Everything should be expired
	})
}

func TestOldReads(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		timeNow := model.TimeAsEpochMicroseconds(time.Now())
		s1Key := createIndexKey(serviceNameIndexKey, []byte("service1"), timeNow, model.TraceID{High: 0, Low: 0})
		s1o1Key := createIndexKey(operationNameIndexKey, []byte("service1operation1"), timeNow, model.TraceID{High: 0, Low: 0})

		tid := time.Now().Add(1 * time.Minute)

		writer := func() {
			store.Update(func(txn *badger.Txn) error {
				txn.SetEntry(&badger.Entry{
					Key:       s1Key,
					ExpiresAt: uint64(tid.Unix()),
				})
				txn.SetEntry(&badger.Entry{
					Key:       s1o1Key,
					ExpiresAt: uint64(tid.Unix()),
				})
				return nil
			})
		}

		cache := NewCacheStore(store, time.Duration(-1*time.Hour), false)
		writer()

		nuTid := tid.Add(1 * time.Hour)

		cache.Update("service1", "operation1", uint64(tid.Unix()))
		cache.services["service1"] = uint64(nuTid.Unix())
		cache.operations["service1"]["operation1"] = uint64(nuTid.Unix())

		cache.populateCaches()

		// Now make sure we didn't use the older timestamps from the DB
		assert.Equal(t, uint64(nuTid.Unix()), cache.services["service1"])
		assert.Equal(t, uint64(nuTid.Unix()), cache.operations["service1"]["operation1"])
	})
}

// func runFactoryTest(tb testing.TB, test func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader)) {
func runWithBadger(t *testing.T, test func(store *badger.DB, t *testing.T)) {
	opts := badger.DefaultOptions("")

	opts.SyncWrites = false
	dir, _ := ioutil.TempDir("", "badger")
	opts.Dir = dir
	opts.ValueDir = dir

	store, err := badger.Open(opts)
	defer func() {
		store.Close()
		os.RemoveAll(dir)
	}()

	assert.NoError(t, err)

	test(store, t)
}
