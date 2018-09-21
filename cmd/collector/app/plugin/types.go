package factory

import (
	"reflect"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/pkg/factory"
)

// Known symbols
const (
	preProcessSymbol = "PreProcess"
	spanFilterSymbol = "SpanFilter"
	sanitizerSymbol  = "Sanitizer"
	preSaveSymbol    = "PreSave"
)

// Used to assert type
var (
	preProcessFunc app.ProcessSpans       = func(spans []*model.Span) {}
	spanFilterFunc app.FilterSpan         = func(span *model.Span) bool { return true }
	sanitizerFunc  sanitizer.SanitizeSpan = func(span *model.Span) *model.Span { return span }
	preSaveFunc    app.ProcessSpan        = func(span *model.Span) {}
)

func PreProcess(pf factory.PluginFactory) (app.ProcessSpans, error) {
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

func SpanFilter(pf factory.PluginFactory) (app.FilterSpan, error) {
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

func Sanitizer(pf factory.PluginFactory) (sanitizer.SanitizeSpan, error) {
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

func PreSave(pf factory.PluginFactory) (app.ProcessSpan, error) {
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
