// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"
	"errors"
	"expvar"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	depstore "github.com/jaegertracing/jaeger/internal/storage/v1/badger/dependencystore"
	badgersampling "github.com/jaegertracing/jaeger/internal/storage/v1/badger/samplingstore"
	badgerstore "github.com/jaegertracing/jaeger/internal/storage/v1/badger/spanstore"
)

const (
	valueLogSpaceAvailableName = "badger_value_log_bytes_available"
	keyLogSpaceAvailableName   = "badger_key_log_bytes_available"
	lastMaintenanceRunName     = "badger_storage_maintenance_last_run"
	lastValueLogCleanedName    = "badger_storage_valueloggc_last_run"
)

var ( // interface comformance checks
	_ io.Closer                    = (*Factory)(nil)
	_ storage.Purger               = (*Factory)(nil)
	_ storage.SamplingStoreFactory = (*Factory)(nil)
)

// Factory for Badger backend.
type Factory struct {
	Config         *Config
	store          *badger.DB
	cache          *badgerstore.CacheStore
	logger         *zap.Logger
	metricsFactory metrics.Factory

	tmpDir          string
	maintenanceDone chan bool
	bgWg            sync.WaitGroup

	// TODO initialize via reflection; convert comments to tag 'description'.
	metrics struct {
		// ValueLogSpaceAvailable returns the amount of space left on the value log mount point in bytes
		ValueLogSpaceAvailable metrics.Gauge
		// KeyLogSpaceAvailable returns the amount of space left on the key log mount point in bytes
		KeyLogSpaceAvailable metrics.Gauge
		// LastMaintenanceRun stores the timestamp (UnixNano) of the previous maintenanceRun
		LastMaintenanceRun metrics.Gauge
		// LastValueLogCleaned stores the timestamp (UnixNano) of the previous ValueLogGC run
		LastValueLogCleaned metrics.Gauge

		// Expose badger's internal expvar metrics, which are all gauge's at this point
		badgerMetrics map[string]metrics.Gauge
	}
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		Config:          DefaultConfig(),
		maintenanceDone: make(chan bool),
	}
}

// Initialize performs internal initialization of the factory.
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.logger = logger
	f.metricsFactory = metricsFactory

	opts := badger.DefaultOptions("")

	if f.Config.Ephemeral {
		opts.SyncWrites = false
		// Error from TempDir is ignored to satisfy Codecov
		dir, _ := os.MkdirTemp("", "badger")
		f.tmpDir = dir
		opts.Dir = f.tmpDir
		opts.ValueDir = f.tmpDir

		f.Config.Directories.Keys = f.tmpDir
		f.Config.Directories.Values = f.tmpDir
	} else {
		// Errors are ignored as they're caught in the Open call
		initializeDir(f.Config.Directories.Keys)
		initializeDir(f.Config.Directories.Values)

		opts.SyncWrites = f.Config.SyncWrites
		opts.Dir = f.Config.Directories.Keys
		opts.ValueDir = f.Config.Directories.Values

		// These options make no sense with ephemeral data
		opts.ReadOnly = f.Config.ReadOnly
	}

	store, err := badger.Open(opts)
	if err != nil {
		return err
	}
	f.store = store

	f.cache = badgerstore.NewCacheStore(f.store, f.Config.TTL.Spans)

	f.metrics.ValueLogSpaceAvailable = metricsFactory.Gauge(metrics.Options{Name: valueLogSpaceAvailableName})
	f.metrics.KeyLogSpaceAvailable = metricsFactory.Gauge(metrics.Options{Name: keyLogSpaceAvailableName})
	f.metrics.LastMaintenanceRun = metricsFactory.Gauge(metrics.Options{Name: lastMaintenanceRunName})
	f.metrics.LastValueLogCleaned = metricsFactory.Gauge(metrics.Options{Name: lastValueLogCleanedName})

	f.registerBadgerExpvarMetrics(metricsFactory)

	f.bgWg.Add(2)
	go func() {
		defer f.bgWg.Done()
		f.maintenance()
	}()
	go func() {
		defer f.bgWg.Done()
		f.metricsCopier()
	}()

	logger.Info("Badger storage configuration", zap.Any("configuration", opts))

	return nil
}

// initializeDir makes the directory and parent directories if the path doesn't exists yet.
func initializeDir(path string) {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		os.MkdirAll(path, 0o700)
	}
}

// CreateSpanReader creates a spanstore.Reader.
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	tr := badgerstore.NewTraceReader(f.store, f.cache, true)
	return spanstoremetrics.NewReaderDecorator(tr, f.metricsFactory), nil
}

// CreateSpanWriter creates a spanstore.Writer.
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return badgerstore.NewSpanWriter(f.store, f.cache, f.Config.TTL.Spans), nil
}

// CreateDependencyReader creates a dependencystore.Reader.
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	sr, _ := f.CreateSpanReader() // err is always nil
	return depstore.NewDependencyStore(sr), nil
}

// CreateSamplingStore implements storage.SamplingStoreFactory
func (f *Factory) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	return badgersampling.NewSamplingStore(f.store), nil
}

// CreateLock implements storage.SamplingStoreFactory
func (*Factory) CreateLock() (distributedlock.Lock, error) {
	return &lock{}, nil
}

// Close Implements io.Closer and closes the underlying storage
func (f *Factory) Close() error {
	close(f.maintenanceDone)
	f.bgWg.Wait() // Wait for background goroutines to finish before closing store
	if f.store == nil {
		return nil
	}
	err := f.store.Close()

	// Remove tmp files if this was ephemeral storage
	if f.Config.Ephemeral {
		errSecondary := os.RemoveAll(f.tmpDir)
		if err == nil {
			err = errSecondary
		}
	}

	return err
}

// Maintenance starts a background maintenance job for the badger K/V store, such as ValueLogGC
func (f *Factory) maintenance() {
	maintenanceTicker := time.NewTicker(f.Config.MaintenanceInterval)
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
			if errors.Is(err, badger.ErrNoRewrite) {
				f.metrics.LastValueLogCleaned.Update(t.UnixNano())
			} else {
				f.logger.Error("Failed to run ValueLogGC", zap.Error(err))
			}

			f.metrics.LastMaintenanceRun.Update(t.UnixNano())
			_ = f.diskStatisticsUpdate()
		}
	}
}

func (f *Factory) metricsCopier() {
	metricsTicker := time.NewTicker(f.Config.MetricsUpdateInterval)
	defer metricsTicker.Stop()
	for {
		select {
		case <-f.maintenanceDone:
			return
		case <-metricsTicker.C:
			expvar.Do(func(kv expvar.KeyValue) {
				if strings.HasPrefix(kv.Key, "badger") {
					switch val := kv.Value.(type) {
					case *expvar.Int:
						if g, found := f.metrics.badgerMetrics[kv.Key]; found {
							g.Update(val.Value())
						}
					case *expvar.Map:
						val.Do(func(innerKv expvar.KeyValue) {
							// The metrics we're interested in have only a single inner key (dynamic name)
							// and we're only interested in its value
							if intVal, ok := innerKv.Value.(*expvar.Int); ok {
								if g, found := f.metrics.badgerMetrics[kv.Key]; found {
									g.Update(intVal.Value())
								}
							}
						})
					default:
						f.logger.Debug("skipping non-numeric badger expvar metric", zap.String("key", kv.Key))
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
			switch val := kv.Value.(type) {
			case *expvar.Int:
				g := metricsFactory.Gauge(metrics.Options{Name: kv.Key})
				f.metrics.badgerMetrics[kv.Key] = g
			case *expvar.Map:
				val.Do(func(innerKv expvar.KeyValue) {
					// The metrics we're interested in have only a single inner key (dynamic name)
					// and we're only interested in its value
					if _, ok := innerKv.Value.(*expvar.Int); ok {
						g := metricsFactory.Gauge(metrics.Options{Name: kv.Key})
						f.metrics.badgerMetrics[kv.Key] = g
					}
				})
			default:
				f.logger.Info("skipping non-numeric badger expvar metric", zap.String("key", kv.Key))
			}
		}
	})
}

// Purge removes all data from the Factory's underlying Badger store.
// This function is intended for testing purposes only and should not be used in production environments.
// Calling Purge in production will result in permanent data loss.
func (f *Factory) Purge(_ context.Context) error {
	return f.store.Update(func(_ *badger.Txn) error {
		return f.store.DropAll()
	})
}
