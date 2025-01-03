package renderer

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
	"go.uber.org/zap"
)

func GenerateESMappings(logger *zap.Logger, options app.Options) error {
	if _, error := mappings.MappingTypeFromString(options.Mapping); error != nil {
		return fmt.Errorf("please pass either 'jaeger-service' or 'jaeger-span' as argument: %W", error)
	}

	parsedMapping, err := GetMappingAsString(es.TextTemplateBuilder{}, &options)
	if err != nil {
		return fmt.Errorf("failed to generate mappings: %w", err)
	}

	fmt.Println(parsedMapping)
	return nil
}
