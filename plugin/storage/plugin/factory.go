package plugin

import (
	"errors"
	"flag"
	"github.com/jaegertracing/jaeger/pkg/pluginloader"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"reflect"
)

// Known symbols
const (
	storageFactorySymbol = "StorageFactory"
)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	options              Options
	metricsFactory       metrics.Factory
	logger               *zap.Logger
	loadedStorageFactory storage.Factory
	pluginLoader         pluginloader.PluginLoader
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper) {
	f.options.InitFromViper(v)
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger

	return nil
}

func (f *Factory) InitializePlugin(pluginLoader pluginloader.PluginLoader) error {
	f.pluginLoader = pluginLoader;

	s, err := f.loadPluginFactory(storageFactorySymbol, reflect.TypeOf(f.loadedStorageFactory))
	if err != nil {
		return err
	}
	f.loadedStorageFactory = s.(storage.Factory)
	return f.loadedStorageFactory.Initialize(f.metricsFactory, f.logger)
}

func (f *Factory) loadPluginFactory(symbol string, expectedType reflect.Type) (interface{}, error) {
	plugins, err := f.pluginLoader.Get(symbol, expectedType)
	if err != nil {
		return nil, err
	}
	if len(plugins) > 1 {
		return nil, errors.New("Running multiple plugin storage is not supported, found multiple "+ symbol +" symbols")
	}
	return plugins[0], nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return f.loadedStorageFactory.CreateSpanReader()
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return f.loadedStorageFactory.CreateSpanWriter()
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return f.loadedStorageFactory.CreateDependencyReader()
}