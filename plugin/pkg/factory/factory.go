package factory

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"reflect"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type factory struct {
	pluginDir string
	logger    *zap.Logger
	plugins   map[string]*plugin.Plugin
}

type PluginFactory interface {
	Initialize() error
	Get(symbol string, expectedType reflect.Type) ([]interface{}, error)
}

func NewPluginFactory(pluginDir string, logger *zap.Logger) PluginFactory {
	return &factory{pluginDir: pluginDir, logger: logger, plugins: make(map[string]*plugin.Plugin)}
}

func (f *factory) Initialize() error {
	if f.pluginDir != "" {
		err := filepath.Walk(f.pluginDir, f.walkpluginDir)
		return err
	}
	return nil
}

func (f *factory) walkpluginDir(path string, info os.FileInfo, err error) error {
	if err != nil {
		return errors.WithMessage(err, "Error walking plugin directory")
	}
	// plugin are compiled as .so file
	if !info.IsDir() && filepath.Ext(path) == ".so" {
		f.logger.Debug("Collector plugin found", zap.String("path", path))
		return f.loadPlugin(path)
	}
	return nil
}

func (f *factory) loadPlugin(path string) error {
	p, err := plugin.Open(path)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("Error while loading plugin at %s", path))
	}
	f.plugins[path] = p
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
			if !foundType.AssignableTo(expected) {
				f.logger.Error("Incompatible type when loading plugin",
					zap.Any("type", reflect.TypeOf(found)),
					zap.Any("expected-type", expected),
					zap.String("symbol", symbol),
					zap.String("path", path))
				return nil, errors.Errorf("Incompatible type when loading plugin")
			}
			results = append(results, found)
			f.logger.Info("Plugin was successfully loaded", zap.String("plugin", filepath.Base(path)), zap.String("symbol", symbol))
		}
	}
	return results, nil
}
