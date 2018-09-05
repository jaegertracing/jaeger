package plugin

import (
	"plugin"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/pkg/errors"
	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"go.uber.org/zap"
	"path/filepath"
)

const (
	preProcessSpansSymbol	= "PreProcessSpans"
	spanFilterSymbol		= "SpanFilter"
	sanitizerSymbol			= "Sanitizer"
	preSaveSymbol			= "PreSave"
)

var symbols = []string{ preProcessSpansSymbol, spanFilterSymbol, sanitizerSymbol, preSaveSymbol }

func incompatibleType(symbol string, path string) error {
	return errors.Errorf("Incompatible type when loading symbol %s from %s", symbol, path)
}

func resolvePluginType(f *factory, p *plugin.Plugin, path string) error {
	var found bool
	for _, s := range symbols {
		// Lookup every known symbols, validate the type and keep track of them
		symbol, err := p.Lookup(s)
		if err == nil {
			switch s {
			case preProcessSpansSymbol:
				ps, ok := symbol.(*app.ProcessSpans)
				if !ok {
					return incompatibleType(s, path)
				}
				f.preProcessors = append(f.preProcessors, *ps)
			case spanFilterSymbol:
				fs, ok := symbol.(*app.FilterSpan)
				if !ok {
					return incompatibleType(s, path)
				}
				f.spanFilters = append(f.spanFilters, *fs)
			case sanitizerSymbol:
				ss, ok := symbol.(*sanitizer.SanitizeSpan)
				if !ok {
					return incompatibleType(s, path)
				}
				f.sanitizers = append(f.sanitizers, *ss)
			case preSaveSymbol:
				ps, ok := symbol.(*app.ProcessSpan)
				if !ok {
					return incompatibleType(s, path)
				}
				f.preSaveProcessors = append(f.preSaveProcessors, *ps)
			}
			f.logger.Info("Plugin was successfully loaded", zap.String("plugin", filepath.Base(path)), zap.String("symbol", s))
			found = true
		}
	}
	if !found {
		return errors.Errorf("Could not find any known symbols in %s", path)
	}
	return nil;
}