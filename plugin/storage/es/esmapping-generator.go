package es

import (
	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app/renderer"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func NewESMappingsCommand(logger *zap.Logger) *cobra.Command {
	options := app.Options{}

	command := &cobra.Command{
		Use:   "es-mappings",
		Short: "Generate Elasticsearch mappings",
		Long:  "Generate Elasticsearch mappings using jaeger-esmapping-generator functionality",
		Run: func(_ *cobra.Command, _ []string) {
			if err := renderer.GenerateESMappings(logger, options); err != nil {
				logger.Fatal("failed to generate mappings: " + err.Error())
			}
		},
	}

	command.Flags().StringVar(&options.Mapping, "mapping", "jaeger-span", "Mapping type to use, e.g. 'jaeger-service' or 'jaeger-span'")
	return command
}
