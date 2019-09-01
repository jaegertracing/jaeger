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
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

const (
	txIDSeqKey  byte = 0x01
	queuePrefix byte = 0x10
	lockPrefix  byte = 0xFF
)

// BadgerOptions includes all the parameters for the underlying badger instance
type BadgerOptions struct {
	Directory   string
	SyncWrites  bool
	Concurrency int
}

type consumerMessage struct {
	batch model.Batch
	txID  uint64
}

// BadgerQueue is persistent backend for the queued reporter
type BadgerQueue struct {
	queueCount   int32
	store        *badger.DB
	logger       *zap.Logger
	txIDGen      *badger.Sequence
	processor    func(model.Batch) (bool, error)
	receiverChan chan consumerMessage
	closeChan    chan struct{}
	ackChan      chan uint64
	notifyChan   chan bool
}

// NewBadgerQueue returns
func NewBadgerQueue(bopts *BadgerOptions, processor func(model.Batch) (bool, error), logger *zap.Logger) (*BadgerQueue, error) {
	opts := badger.DefaultOptions
	opts.TableLoadingMode = options.MemoryMap
	opts.Dir = bopts.Directory
	opts.ValueDir = bopts.Directory
	opts.SyncWrites = bopts.SyncWrites

	if processor == nil {
		return nil, fmt.Errorf("no processor defined, could not initialize queue")
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	seq, err := db.GetSequence([]byte{txIDSeqKey}, 1000)
	if err != nil {
		db.Close()
		return nil, err
	}

	b := &BadgerQueue{
		logger:       logger,
		store:        db,
		txIDGen:      seq,
		ackChan:      make(chan uint64, bopts.Concurrency),
		receiverChan: make(chan consumerMessage, bopts.Concurrency),
		closeChan:    make(chan struct{}),
		notifyChan:   make(chan bool, 1),
		processor:    processor,
	}

	// Remove all the lock keys here and calculate current size - the process has restarted
	initialCount, err := b.initializeQueue()
	if err != nil {
		db.Close()
		return nil, err
	}
	b.queueCount = initialCount

	// Start ack processing
	go b.acker()
	// Start dequeue processing
	go b.dequeuer()
	// Start processing items
	for i := 0; i < bopts.Concurrency; i++ {
		go b.batchProcessor()
	}

	// Wake up the dequeuer
	b.notifyChan <- true

	// TODO Start maintenance and other.. as soon as these are in the common place
	/*
		f.registerBadgerExpvarMetrics(metricsFactory)

		go f.maintenance()
		go f.metricsCopier()
	*/
	// TODO Add metrics (queued items, dequeued items, queue size, average queue waiting time etc..)
	return b, nil
}

func (b *BadgerQueue) batchProcessor() {
	for {
		select {
		case cm := <-b.receiverChan:
			// Any error is not retryable and has nothing to do with queue implementation..
			delivered, _ := b.processor(cm.batch)
			if delivered {
				b.ackChan <- cm.txID
			}
		case <-b.closeChan:
			return
		}
	}
}

// Enqueue stores the batch to the Badger's storage
func (b *BadgerQueue) Enqueue(batch model.Batch) error {
	txID, err := b.nextTransactionID()
	if err != nil {
		return err
	}

	key := make([]byte, 9)
	lockKey := make([]byte, 9)
	key[0] = queuePrefix
	lockKey[0] = lockPrefix
	binary.BigEndian.PutUint64(key[1:], txID)
	binary.BigEndian.PutUint64(lockKey[1:], txID)

	bypassQueue := false

	/*
		if int(atomic.LoadInt32(&b.queueCount)) < cap(b.receiverChan) {
			// Not really a too good algorithm..
			bypassQueue = true
		}
	*/

	bb, err := proto.Marshal(&batch)
	if err != nil {
		return err
	}

	err = b.store.Update(func(txn *badger.Txn) error {
		/*
			if bypassQueue {
				err := txn.SetWithTTL(lockKey, nil, b.getLockTime())
				if err != nil {
					return err
				}
			}
		*/
		return txn.Set(key, bb)
	})
	atomic.AddInt32(&b.queueCount, int32(1))
	/*if err == nil && bypassQueue {
		// Push to channel since there's space - no point passing it through badger
		b.receiverChan <- consumerMessage{
			txID:  txID,
			batch: batch,
		}
	} else */
	if err == nil && !bypassQueue {
		// Notify the dequeuer that new stuff has been added (if it is sleeping)
		select {
		case b.notifyChan <- true:
		default:
		}
	}
	return err
}

func (b *BadgerQueue) nextTransactionID() (uint64, error) {
	return b.txIDGen.Next()
}

// getLockTime calculates the proper timeout for processed item
func (b *BadgerQueue) getLockTime() time.Duration {
	return time.Hour
}

// dequeuer runs in the background and pushes data to the channel whenever there's available space.
// This is done to push racing of concurrent processors to the chan implementation instead of badger.
// However, this method should stay concurrent safe (if there's need to add more loaders). To provide
// that safety, we read the lock key before writing it - this causes a transaction error with
// snapshot isolation.
func (b *BadgerQueue) dequeuer() {
	for {
		select {
		case <-b.notifyChan:
			// Process all the items in the queue
			err := b.dequeueAvailableItems()
			if err != nil {
				b.logger.Error("Could not dequeue next item from badger", zap.Error(err))
			}
		case <-b.closeChan:
			return
		}
	}
}

func parseItem(item *badger.Item) (consumerMessage, error) {
	txID := binary.BigEndian.Uint64(item.Key()[1:])
	value := []byte{}
	value, err := item.ValueCopy(value)
	if err != nil {
		return consumerMessage{}, err
	}

	batch := model.Batch{}

	if err := proto.Unmarshal(value, &batch); err != nil {
		return consumerMessage{}, err
	}

	cm := consumerMessage{
		txID:  txID,
		batch: batch,
	}

	return cm, nil
}

// dequeueAvailableItems fetches items from the queue until there's no more to be fetched
// and places them to the receiverChan for processing
func (b *BadgerQueue) dequeueAvailableItems() error {
	for {
		fetchedItems := make([]consumerMessage, 0, cap(b.receiverChan))

		err := b.store.Update(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			it := txn.NewIterator(opts)
			defer it.Close()

			prefix := []byte{queuePrefix}

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				lockKey := make([]byte, len(item.Key()))
				copy(lockKey, item.Key())
				lockKey[0] = lockPrefix

				if _, err := txn.Get(lockKey); err == badger.ErrKeyNotFound {
					// No lockKey present, we can take this item
					err2 := txn.SetWithTTL(lockKey, nil, b.getLockTime())
					if err2 != nil {
						return err2
					}
					cm, err := parseItem(item)
					if err != nil {
						return err
					}
					fetchedItems = append(fetchedItems, cm)
					if len(fetchedItems) == cap(fetchedItems) {
						// Don't fetch more items at once
						return nil
					}
				}
			}
			return nil
		})

		if err != nil {
			if err == badger.ErrConflict {
				// Concurrent read/Write occurred, our transaction lost. Try again.
				continue
			}
			return err
		}

		for _, cm := range fetchedItems {
			b.receiverChan <- cm
		}

		// We cleared the queue during this transaction, back to sleep
		if len(fetchedItems) < cap(fetchedItems) {
			return nil
		}
	}
}

// acker receives acknowledgements transactionIds from the ackChan and delete them from badger
func (b *BadgerQueue) acker() {
	for {
		select {
		case id := <-b.ackChan:
			key := make([]byte, 9)
			lockKey := make([]byte, 9)
			key[0] = queuePrefix
			lockKey[0] = lockPrefix
			binary.BigEndian.PutUint64(key[1:], id)
			binary.BigEndian.PutUint64(lockKey[1:], id)

			err := b.store.Update(func(txn *badger.Txn) error {
				txn.Delete(key)
				txn.Delete(lockKey)
				return nil
			})

			if err != nil {
				b.logger.Error("Could not remove item from the queue", zap.Error(err))
			}
			atomic.AddInt32(&b.queueCount, int32(-1))
		case <-b.closeChan:
			return
		}
	}
}

// Close stops processing items from the queue and closes the underlying storage
func (b *BadgerQueue) Close() error {
	close(b.closeChan)
	err := b.store.Close()
	return err
}

func (b *BadgerQueue) initializeQueue() (int32, error) {
	// Remove all lock keys, set the correct queue size
	err := b.store.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte{lockPrefix}

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := txn.Delete(it.Item().Key())
			if err != nil {
				return err
			}
		}

		return nil
	})

	return 0, err
}

// TODO Add to the previous ones.. max retries option (with requeue or ditch) - sort of like DLQ so that one object doesn't close the whole processing
// if one item is broken

// TODO Could these be in some common repository for all badger parts, this one as well as the main storage
/*
// Maintenance starts a background maintenance job for the badger K/V store, such as ValueLogGC
func (f *Factory) maintenance() {
	maintenanceTicker := time.NewTicker(f.Options.primary.MaintenanceInterval)
	defer maintenanceTicker.Stop()
	for {
		select {
		case <-f.maintenanceDone:
			return
		case t := <-maintenanceTicker.C:
			var err error

			// After there's nothing to clean, the err is raised
			for err == nil {
				err = f.store.RunValueLogGC(0.5) // 0.5 is selected to rewrite a file if half of it can be discarded
			}
			if err == badger.ErrNoRewrite {
				f.metrics.LastValueLogCleaned.Update(t.UnixNano())
			} else {
				f.logger.Error("Failed to run ValueLogGC", zap.Error(err))
			}

			f.metrics.LastMaintenanceRun.Update(t.UnixNano())
			f.diskStatisticsUpdate()
		}
	}
}

func (f *Factory) metricsCopier() {
	metricsTicker := time.NewTicker(f.Options.primary.MetricsUpdateInterval)
	defer metricsTicker.Stop()
	for {
		select {
		case <-f.maintenanceDone:
			return
		case <-metricsTicker.C:
			expvar.Do(func(kv expvar.KeyValue) {
				if strings.HasPrefix(kv.Key, "badger") {
					if intVal, ok := kv.Value.(*expvar.Int); ok {
						if g, found := f.metrics.badgerMetrics[kv.Key]; found {
							g.Update(intVal.Value())
						}
					} else if mapVal, ok := kv.Value.(*expvar.Map); ok {
						mapVal.Do(func(innerKv expvar.KeyValue) {
							// The metrics we're interested in have only a single inner key (dynamic name)
							// and we're only interested in its value
							if intVal, ok := innerKv.Value.(*expvar.Int); ok {
								if g, found := f.metrics.badgerMetrics[kv.Key]; found {
									g.Update(intVal.Value())
								}
							}
						})
					}
				}
			})
		}
	}
}

func (f *Factory) registerBadgerExpvarMetrics(metricsFactory metrics.Factory) {
	f.metrics.badgerMetrics = make(map[string]metrics.Gauge)

	expvar.Do(func(kv expvar.KeyValue) {
		if strings.HasPrefix(kv.Key, "badger") {
			if _, ok := kv.Value.(*expvar.Int); ok {
				g := metricsFactory.Gauge(metrics.Options{Name: kv.Key})
				f.metrics.badgerMetrics[kv.Key] = g
			} else if mapVal, ok := kv.Value.(*expvar.Map); ok {
				mapVal.Do(func(innerKv expvar.KeyValue) {
					// The metrics we're interested in have only a single inner key (dynamic name)
					// and we're only interested in its value
					if _, ok = innerKv.Value.(*expvar.Int); ok {
						g := metricsFactory.Gauge(metrics.Options{Name: kv.Key})
						f.metrics.badgerMetrics[kv.Key] = g
					}
				})
			}
		}
	})
}
*/
