// Copyright (c) 2020 The Jaeger Authors.
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

package resourceprocessor

import (
	"context"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/collector/processor/resourceprocessor"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/flags"
)

const (
	resourceLabels = "resource.attributes"
)

// Factory wraps resourceprocessor.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	Wrapped component.ProcessorFactory
	Viper   *viper.Viper
}

var _ component.ProcessorFactory = (*Factory)(nil)

// Type returns the type of the receiver.
func (f Factory) Type() configmodels.Type {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ProcessorFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Processor {
	return f.Wrapped.CreateDefaultConfig()
}

// GetTags returns tags to be added to all spans.
func (f Factory) GetTags() map[string]string {
	tagsLegacy := flags.ParseJaegerTags(f.Viper.GetString(reporter.AgentTagsDeprecated))
	tags := flags.ParseJaegerTags(f.Viper.GetString(resourceLabels))
	if tags == nil {
		return tagsLegacy
	}
	for k, v := range tagsLegacy {
		if _, ok := tags[k]; !ok {
			tags[k] = v
		}
	}
	return tags
}

// CreateTracesProcessor creates resource processor.
// This function implements OTEL component.ProcessorFactoryOld interface.
func (f Factory) CreateTracesProcessor(
	ctx context.Context,
	params component.ProcessorCreateParams,
	cfg configmodels.Processor,
	nextConsumer consumer.TracesConsumer,
) (component.TracesProcessor, error) {
	c := cfg.(*resourceprocessor.Config)
	attributeKeys := map[string]bool{}
	for _, kv := range c.AttributesActions {
		attributeKeys[kv.Key] = true
	}
	for k, v := range f.GetTags() {
		// do not override values in OTEL config.
		// OTEL config has higher precedence
		if !attributeKeys[k] {
			c.AttributesActions = append(c.AttributesActions, processorhelper.ActionKeyValue{
				Key:    k,
				Value:  v,
				Action: processorhelper.UPSERT,
			})
		}
	}
	return f.Wrapped.CreateTracesProcessor(ctx, params, cfg, nextConsumer)
}

// CreateMetricsProcessor creates a resource processor.
// This function implements component.ProcessorFactoryOld.
func (f Factory) CreateMetricsProcessor(
	ctx context.Context,
	params component.ProcessorCreateParams,
	cfg configmodels.Processor,
	nextConsumer consumer.MetricsConsumer,
) (component.MetricsProcessor, error) {
	return f.Wrapped.CreateMetricsProcessor(ctx, params, cfg, nextConsumer)
}

// CreateLogsProcessor creates a processor based on the config.
// If the processor type does not support logs or if the config is not valid
// error will be returned instead.
func (f Factory) CreateLogsProcessor(
	ctx context.Context,
	params component.ProcessorCreateParams,
	cfg configmodels.Processor,
	nextConsumer consumer.LogsConsumer,
) (component.LogsProcessor, error) {
	return f.Wrapped.CreateLogsProcessor(ctx, params, cfg, nextConsumer)
}
