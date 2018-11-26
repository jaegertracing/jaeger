package factory

import (
	"reflect"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/pkg/pluginloader"
)

// Known symbols
const (
	preProcessSymbol = "PreProcess"
	spanFilterSymbol = "SpanFilter"
	sanitizerSymbol  = "Sanitizer"
	preSaveSymbol    = "PreSave"
)

// Used only for assert type
var (
	preProcessFunc app.ProcessSpans
	spanFilterFunc app.FilterSpan
	sanitizerFunc  sanitizer.SanitizeSpan
	preSaveFunc    app.ProcessSpan
)

func PreProcess(pf pluginloader.PluginLoader) (app.ProcessSpans, error) {
	if pf == nil {
		return nil, nil
	}

	plugins, err := pf.Get(preProcessSymbol, reflect.TypeOf(preProcessFunc))
	if err != nil {
		return nil, err
	}
	switch len(plugins) {
	case 0:
		return nil, nil
	case 1:
		return plugins[0].(app.ProcessSpans), nil
	default:
		toChain := make([]app.ProcessSpans, len(plugins))
		for _, p := range plugins {
			toChain = append(toChain, p.(app.ProcessSpans))
		}
		return app.ChainedProcessSpans(toChain...), nil
	}
}

func SpanFilter(pf pluginloader.PluginLoader) (app.FilterSpan, error) {
	if pf == nil {
		return nil, nil
	}

	plugins, err := pf.Get(spanFilterSymbol, reflect.TypeOf(spanFilterFunc))
	if err != nil {
		return nil, err
	}
	switch len(plugins) {
	case 0:
		return nil, nil
	case 1:
		return plugins[0].(app.FilterSpan), nil
	default:
		toChain := make([]app.FilterSpan, len(plugins))
		for _, p := range plugins {
			toChain = append(toChain, p.(app.FilterSpan))
		}
		return app.ChainedFilterSpan(toChain...), nil
	}
}

func Sanitizer(pf pluginloader.PluginLoader) (sanitizer.SanitizeSpan, error) {
	if pf == nil {
		return nil, nil
	}

	plugins, err := pf.Get(sanitizerSymbol, reflect.TypeOf(sanitizerFunc))
	if err != nil {
		return nil, err
	}
	switch len(plugins) {
	case 0:
		return nil, nil
	case 1:
		return plugins[0].(sanitizer.SanitizeSpan), nil
	default:
		toChain := make([]sanitizer.SanitizeSpan, len(plugins))
		for _, p := range plugins {
			toChain = append(toChain, p.(sanitizer.SanitizeSpan))
		}
		return sanitizer.NewChainedSanitizer(toChain...), nil
	}
}

func PreSave(pf pluginloader.PluginLoader) (app.ProcessSpan, error) {
	if pf == nil {
		return nil, nil
	}

	plugins, err := pf.Get(preSaveSymbol, reflect.TypeOf(preSaveFunc))
	if err != nil {
		return nil, err
	}
	switch len(plugins) {
	case 0:
		return nil, nil
	case 1:
		return plugins[0].(app.ProcessSpan), nil
	default:
		toChain := make([]app.ProcessSpan, len(plugins))
		for _, p := range plugins {
			toChain = append(toChain, p.(app.ProcessSpan))
		}
		return app.ChainedProcessSpan(toChain...), nil
	}
}
