// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"
	"errors"
	"expvar"
	"flag"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	depStore "github.com/jaegertracing/jaeger/plugin/storage/badger/dependencystore"
	badgerSampling "github.com/jaegertracing/jaeger/plugin/storage/badger/samplingstore"
	badgerStore "github.com/jaegertracing/jaeger/plugin/storage/badger/spanstore"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

const (
	valueLogSpaceAvailableName = "badger_value_log_bytes_available"
	keyLogSpaceAvailableName   = "badger_key_log_bytes_available"
	lastMaintenanceRunName     = "badger_storage_maintenance_last_run"
	lastValueLogCleanedName    = "badger_storage_valueloggc_last_run"
)

var ( // interface comformance checks
	_ storage.Factory     = (*Factory)(nil)
	_ io.Closer           = (*Factory)(nil)
	_ plugin.Configurable = (*Factory)(nil)
	_ storage.Purger      = (*Factory)(nil)

	// TODO badger could implement archive storage
	// _ storage.ArchiveFactory       = (*Factory)(nil)

	_ storage.SamplingStoreFactory = (*Factory)(nil)
)

// Factory implements storage.Factory for Badger backend.
type Factory struct {
	Config *Config
	store  *badger.DB
	cache  *badgerStore.CacheStore
	logger *zap.Logger

	tmpDir          string
	maintenanceDone chan bool

	metricsFactory metrics.Factory

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

func NewFactoryWithConfig(
	cfg Config,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	f := NewFactory()
	f.configure(&cfg)
	err := f.Initialize(metricsFactory, logger)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Config.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	f.Config.InitFromViper(v, logger)
	f.configure(f.Config)
}

// configure initializes Factory from supplied Config.
func (f *Factory) configure(config *Config) {
	f.Config = config
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger

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

	f.cache = badgerStore.NewCacheStore(f.store, f.Config.TTL.Spans, true)

	f.metrics.ValueLogSpaceAvailable = metricsFactory.Gauge(metrics.Options{Name: valueLogSpaceAvailableName})
	f.metrics.KeyLogSpaceAvailable = metricsFactory.Gauge(metrics.Options{Name: keyLogSpaceAvailableName})
	f.metrics.LastMaintenanceRun = metricsFactory.Gauge(metrics.Options{Name: lastMaintenanceRunName})
	f.metrics.LastValueLogCleaned = metricsFactory.Gauge(metrics.Options{Name: lastValueLogCleanedName})

	f.registerBadgerExpvarMetrics(metricsFactory)

	go f.maintenance()
	go f.metricsCopier()

	logger.Info("Badger storage configuration", zap.Any("configuration", opts))

	return nil
}

// initializeDir makes the directory and parent directories if the path doesn't exists yet.
func initializeDir(path string) {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		os.MkdirAll(path, 0o700)
	}
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	sr := badgerStore.NewTraceReader(f.store, f.cache)
	return storageMetrics.NewReadMetricsDecorator(sr, f.metricsFactory), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return badgerStore.NewSpanWriter(f.store, f.cache, f.Config.TTL.Spans), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	sr, _ := f.CreateSpanReader() // err is always nil
	return depStore.NewDependencyStore(sr), nil
}

// CreateSamplingStore implements storage.SamplingStoreFactory
func (f *Factory) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	return badgerSampling.NewSamplingStore(f.store), nil
}

// CreateLock implements storage.SamplingStoreFactory
func (*Factory) CreateLock() (distributedlock.Lock, error) {
	return &lock{}, nil
}

// Close Implements io.Closer and closes the underlying storage
func (f *Factory) Close() error {
	close(f.maintenanceDone)
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
			f.diskStatisticsUpdate()
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

// Purge removes all data from the Factory's underlying Badger store.
// This function is intended for testing purposes only and should not be used in production environments.
// Calling Purge in production will result in permanent data loss.
func (f *Factory) Purge(_ context.Context) error {
	return f.store.Update(func(_ *badger.Txn) error {
		return f.store.DropAll()
	})
}
