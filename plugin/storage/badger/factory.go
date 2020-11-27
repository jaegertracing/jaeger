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

package badger

import (
	"expvar"
	"flag"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	depStore "github.com/jaegertracing/jaeger/plugin/storage/badger/dependencystore"
	badgerStore "github.com/jaegertracing/jaeger/plugin/storage/badger/spanstore"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	valueLogSpaceAvailableName = "badger_value_log_bytes_available"
	keyLogSpaceAvailableName   = "badger_key_log_bytes_available"
	lastMaintenanceRunName     = "badger_storage_maintenance_last_run"
	lastValueLogCleanedName    = "badger_storage_valueloggc_last_run"
)

// Factory implements storage.Factory for Badger backend.
type Factory struct {
	Options *Options
	store   *badger.DB
	cache   *badgerStore.CacheStore
	logger  *zap.Logger

	tmpDir          string
	maintenanceDone chan bool

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
		Options:         NewOptions("badger"),
		maintenanceDone: make(chan bool),
	}
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper) {
	f.Options.InitFromViper(v)
}

// InitFromOptions initializes Factory from supplied options
func (f *Factory) InitFromOptions(opts Options) {
	f.Options = &opts
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.logger = logger

	opts := badger.DefaultOptions("")
	opts.TableLoadingMode = options.MemoryMap

	if f.Options.Primary.Ephemeral {
		opts.SyncWrites = false
		// Error from TempDir is ignored to satisfy Codecov
		dir, _ := ioutil.TempDir("", "badger")
		f.tmpDir = dir
		opts.Dir = f.tmpDir
		opts.ValueDir = f.tmpDir

		f.Options.Primary.KeyDirectory = f.tmpDir
		f.Options.Primary.ValueDirectory = f.tmpDir
	} else {
		// Errors are ignored as they're caught in the Open call
		initializeDir(f.Options.Primary.KeyDirectory)
		initializeDir(f.Options.Primary.ValueDirectory)

		opts.SyncWrites = f.Options.Primary.SyncWrites
		opts.Dir = f.Options.Primary.KeyDirectory
		opts.ValueDir = f.Options.Primary.ValueDirectory

		// These options make no sense with ephemeral data
		opts.Truncate = f.Options.Primary.Truncate
		opts.ReadOnly = f.Options.Primary.ReadOnly
	}

	store, err := badger.Open(opts)
	if err != nil {
		return err
	}
	f.store = store

	f.cache = badgerStore.NewCacheStore(f.store, f.Options.Primary.SpanStoreTTL, true)

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
		os.MkdirAll(path, 0700)
	}
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return badgerStore.NewTraceReader(f.store, f.cache), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return badgerStore.NewSpanWriter(f.store, f.cache, f.Options.Primary.SpanStoreTTL), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	sr, _ := f.CreateSpanReader() // err is always nil
	return depStore.NewDependencyStore(sr), nil
}

// Close Implements io.Closer and closes the underlying storage
func (f *Factory) Close() error {
	close(f.maintenanceDone)
	if f.store == nil {
		return nil
	}
	err := f.store.Close()

	// Remove tmp files if this was ephemeral storage
	if f.Options.Primary.Ephemeral {
		errSecondary := os.RemoveAll(f.tmpDir)
		if err == nil {
			err = errSecondary
		}
	}

	return err
}

// Maintenance starts a background maintenance job for the badger K/V store, such as ValueLogGC
func (f *Factory) maintenance() {
	maintenanceTicker := time.NewTicker(f.Options.Primary.MaintenanceInterval)
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
	metricsTicker := time.NewTicker(f.Options.Primary.MetricsUpdateInterval)
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
