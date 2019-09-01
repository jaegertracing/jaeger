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

package queue

import (
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

type receiverProcessor struct {
	wg *sync.WaitGroup
}

func (r *receiverProcessor) Process(batch model.Batch) (bool, error) {
	r.wg.Done()
	return true, nil
}

func TestBadgerDequeue(t *testing.T) {
	assert := assert.New(t)
	dir, _ := ioutil.TempDir("", "badger")
	bo := &BadgerOptions{
		Directory:   dir,
		Concurrency: 1,
	}
	defer os.RemoveAll(dir)

	wg := &sync.WaitGroup{}
	r := receiverProcessor{wg: wg}

	b, err := NewBadgerQueue(bo, r.Process, zap.NewNop())
	assert.NoError(err)

	defer b.Close()

	for i := 0; i < 3; i++ {
		wg.Add(1)
		b.Enqueue(model.Batch{})
	}

	waitChan := make(chan struct{})

	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		break
	case <-time.After(time.Hour * 3):
		assert.Fail("Did not receive 3 messages in time\n")
	}
}

// TestDequeuerConcurrencyCorrectness enqueues new items at random interval with 16 concurrent workers
// for 16 processes and random amount of dequeuing processes. If there's negative waitGroup amount then
// something got processed twice. And timeout means something went missing.
func TestDequeuerConcurrencyCorrectness(t *testing.T) {
	if os.Getenv("BADGER_TORTURE_TEST") == "" {
		// We don't want to run this as default
		t.Skip("Torture test skipped")
	}

	assert := assert.New(t)
	dir, _ := ioutil.TempDir("", "badger")
	bo := &BadgerOptions{
		Directory:   dir,
		Concurrency: 16,
	}
	defer os.RemoveAll(dir)

	wg := &sync.WaitGroup{}
	r := receiverProcessor{wg: wg}

	b, err := NewBadgerQueue(bo, r.Process, zap.NewNop())
	assert.NoError(err)

	runChan := make(chan struct{})

	for i := 0; i < bo.Concurrency; i++ {
		go func() {
			for {
				select {
				case <-runChan:
					return
				default:
				}
				time.Sleep(time.Millisecond * time.Duration(rand.Int63n(100)))
				wg.Add(1)
				b.Enqueue(model.Batch{})
				if rand.Int31n(3) > 2 {
					// Launch random amount of extra dequeuer processes to punish the SSI
					go b.dequeueAvailableItems()
				}
			}
		}()
	}

	time.Sleep(1 * time.Minute) // Let the goroutines run this long
	close(runChan)

	// Wait for all the messages to be processed
	waitChan := make(chan struct{})

	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		break
	case <-time.After(time.Hour * 3):
		assert.Fail("Did not receive messages in time\n")
	}
}

// TestPostgresExample verifies our logic still works with current version of badger
// this example is from Postgres' documentation of how SSI should work
func TestPostgresExample(t *testing.T) {
	assert := assert.New(t)

	dir, _ := ioutil.TempDir("", "badger")
	defer os.RemoveAll(dir)

	opts := badger.DefaultOptions
	opts.TableLoadingMode = options.MemoryMap
	opts.Dir = dir
	opts.ValueDir = dir

	db, err := badger.Open(opts)
	assert.NoError(err)

	// TXN1 progress
	txn1 := db.NewTransaction(true)
	_, err = txn1.Get([]byte("color"))
	if err == badger.ErrKeyNotFound {
		txn1.Set([]byte("color"), []byte("black"))
	}

	// TXN2 progress
	txn2 := db.NewTransaction(true)
	_, err = txn2.Get([]byte("color"))
	if err == badger.ErrKeyNotFound {
		txn2.Set([]byte("color"), []byte("white"))
	}

	// Commit TXN2, because of SSI there should no issues
	err = txn2.Commit(nil)
	assert.NoError(err)

	// Commit TXN1, this should fail because txn1.Get is no longer
	// equal to our assumption
	err = txn1.Commit(nil)
	assert.Error(err)
	assert.EqualError(badger.ErrConflict, err.Error())

	txn3 := db.NewTransaction(true)
	item, err := txn3.Get([]byte("color"))
	assert.NoError(err)

	val, err := item.Value()
	assert.NoError(err)

	// TXN2 value should be returned as it was first committed
	if string(val) != "white" {
		t.Fatal("color should be white")
	}
}

func TestCreateErrors(t *testing.T) {
	assert := assert.New(t)
	bo := &BadgerOptions{
		Directory:   "/root/this_should_fail",
		Concurrency: 1,
	}

	_, err := NewBadgerQueue(bo, nil, zap.NewNop())
	assert.Error(err)

	// wg := &sync.WaitGroup{}
	r := receiverProcessor{}

	_, err = NewBadgerQueue(bo, r.Process, zap.NewNop())
	assert.Error(err)
}
