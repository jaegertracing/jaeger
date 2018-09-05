package plugin

import (
	"os"
	"path/filepath"
	"plugin"
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"go.uber.org/zap"
	"github.com/pkg/errors"
)

type factory struct {
	pluginsDir        string
	preProcessors     []app.ProcessSpans
	spanFilters       []app.FilterSpan
	sanitizers        []sanitizer.SanitizeSpan
	preSaveProcessors []app.ProcessSpan
	logger            *zap.Logger
}

type PluginsFactory interface {
	Load() error
	PreProcessor() app.ProcessSpans
	SpanFilter() app.FilterSpan
	Sanitizer() sanitizer.SanitizeSpan
	PreSaveProcessor() app.ProcessSpan
}

func NewPluginsFactory(pluginsDir string, logger *zap.Logger) PluginsFactory {
	return &factory{ pluginsDir: pluginsDir, logger: logger };
}

func (f *factory) Load() error {
	if f.pluginsDir != "" {
		err := filepath.Walk(f.pluginsDir, f.walkPluginsDir)
		return err;
	}
	return nil;
}

func (f *factory) walkPluginsDir(path string, info os.FileInfo, err error) error {
	if err != nil {
		return errors.WithMessage(err, "Error walking plugins directory")
	}
	// Plugins are compiled as .so file
	if ! info.IsDir() && filepath.Ext(path) == ".so" {
		f.logger.Debug("Collector plugin found", zap.String("path", path))
		return f.loadPlugin(path)
	}
	return nil
}

func (f *factory) loadPlugin(path string) error {
	p, err := plugin.Open(path);
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("Error while loading plugin at %s", path))
	}
	return resolvePluginType(f, p, path)
}

func (f *factory) Sanitizer() sanitizer.SanitizeSpan {
	switch len(f.sanitizers) {
	case 0:
		return nil
	case 1:
		return f.sanitizers[0]
	default:
		return sanitizer.NewChainedSanitizer(f.sanitizers...)
	}
}

func (f *factory) PreProcessor() app.ProcessSpans {
	switch len(f.preProcessors) {
	case 0:
		return nil
	case 1:
		return f.preProcessors[0]
	default:
		return app.ChainedProcessSpans(f.preProcessors...)
	}
}

func (f *factory) PreSaveProcessor() app.ProcessSpan {
	switch len(f.preSaveProcessors) {
	case 0:
		return nil
	case 1:
		return f.preSaveProcessors[0]
	default:
		return app.ChainedProcessSpan(f.preSaveProcessors...)
	}
}

func (f *factory) SpanFilter() app.FilterSpan {
	switch len(f.spanFilters) {
	case 0:
		return nil
	case 1:
		return f.spanFilters[0]
	default:
		return app.ChainedFilterSpan(f.spanFilters...)
	}
}