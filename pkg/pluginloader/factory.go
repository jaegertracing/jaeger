package pluginloader

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	pluginPkg "plugin"
	"reflect"

	"github.com/jaegertracing/jaeger/plugin"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
)

type factory struct {
	pluginsDir string
	logger     *zap.Logger
	plugins    map[string]*pluginPkg.Plugin
}

type PluginLoader interface {
	Load() error
	AddFlags(flagSet *flag.FlagSet)
	InitFromViper(v *viper.Viper)
	Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error
	Get(symbol string, expectedType reflect.Type) ([]interface{}, error)
}

func NewPluginLoader(config FactoryConfig) (PluginLoader, error) {
	if config.PluginsDirectory != "" {
		fi, err := os.Stat(config.PluginsDirectory)
		if os.IsNotExist(err) {
			return nil, errors.Errorf("The provided plugin directory does not exists: %s", config.PluginsDirectory)
		}
		if !fi.IsDir() {
			return nil, errors.Errorf("The provided plugin directory is not a directory: %s", config.PluginsDirectory)
		}
	}
	return &factory{
		pluginsDir: config.PluginsDirectory,
		plugins:    make(map[string]*pluginPkg.Plugin),
		logger:     config.InitialLogger,
	}, nil
}

func (f *factory) Load() error {
	if f.pluginsDir != "" {
		err := filepath.Walk(f.pluginsDir, f.walkpluginDir)
		return err
	}
	return nil
}

func (f *factory) Get(symbol string, expected reflect.Type) ([]interface{}, error) {
	var results []interface{}
	for path, plug := range f.plugins {
		found, err := plug.Lookup(symbol)
		if err != nil {
			f.logger.Debug("Could not find symbol", zap.String("symbol", symbol), zap.String("path", path), zap.Error(err))
		} else {
			foundType := reflect.TypeOf(found)
			if foundType.Kind() == reflect.Ptr {
				foundType = foundType.Elem()
				found = reflect.ValueOf(found).Elem().Interface()
			}
			if !f.isTypeCompatible(found, expected) {
				return nil, errors.Errorf("Incompatible type when loading plugin")
			}
			results = append(results, found)
			f.logger.Info("Plugin was successfully loaded", zap.String("plugin", filepath.Base(path)), zap.String("symbol", symbol))
		}
	}
	return results, nil
}

func (f *factory) AddFlags(flagSet *flag.FlagSet) {
	plugins, _ := f.Get(configurableSymbol, reflect.TypeOf((*plugin.Configurable)(nil)).Elem())
	for _, p := range plugins {
		p.(plugin.Configurable).AddFlags(flagSet)
	}
}

func (f *factory) InitFromViper(v *viper.Viper) {
	plugins, _ := f.Get(configurableSymbol, reflect.TypeOf((*plugin.Configurable)(nil)).Elem())
	for _, p := range plugins {
		p.(plugin.Configurable).InitFromViper(v)
	}
}

func (f *factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.addLogger(logger)
	f.addMetrics(metricsFactory)

	return nil
}

func (f *factory) addMetrics(metricsFactory metrics.Factory) {
	plugins, _ := f.Get(configurableMetricsSymbol, reflect.TypeOf((*plugin.ConfigurableMetrics)(nil)).Elem())
	for _, p := range plugins {
		p.(plugin.ConfigurableMetrics).AddMetrics(metricsFactory)
	}
}

func (f *factory) addLogger(logger *zap.Logger) {
	f.logger = logger

	plugins, _ := f.Get(configurableLoggingSymbol, reflect.TypeOf((*plugin.ConfigurableLogging)(nil)).Elem())
	for _, p := range plugins {
		p.(plugin.ConfigurableLogging).AddLogger(logger)
	}
}

func (f *factory) walkpluginDir(path string, info os.FileInfo, err error) error {
	if err != nil {
		return errors.WithMessage(err, "Error walking plugin directory")
	}
	// plugin are compiled as .so file
	if !info.IsDir() && filepath.Ext(path) == ".so" {
		f.logger.Info("Collector plugin found", zap.String("path", path))
		return f.loadPlugin(path)
	}
	return nil
}

func (f *factory) loadPlugin(path string) error {
	p, err := pluginPkg.Open(path)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("Error while loading plugin at %s", path))
	}
	f.plugins[path] = p
	return nil
}

func (f *factory) isTypeCompatible(actual interface{}, expected reflect.Type) bool {
	foundType := reflect.TypeOf(actual)
	if (expected.Kind() == reflect.Interface && !foundType.Implements(expected)) || !foundType.AssignableTo(expected) {
		f.logger.Error("Incompatible type when loading plugin",
			zap.Any("found-type", reflect.TypeOf(actual)),
			zap.Any("expected-type", expected))
		return false
	}
	return true
}
