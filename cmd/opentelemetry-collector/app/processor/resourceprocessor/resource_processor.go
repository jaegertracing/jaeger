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
	"flag"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer"
	"github.com/open-telemetry/opentelemetry-collector/processor/resourceprocessor"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/flags"
)

const (
	resourceTags       = "resource.tags"
	resourceTagsLegacy = "jaeger.tags"
)

// Factory wraps resourceprocessor.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	Wrapped *resourceprocessor.Factory
	Viper   *viper.Viper
}

var _ component.ProcessorFactoryOld = (*Factory)(nil)

// Type returns the type of the receiver.
func (f Factory) Type() configmodels.Type {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ProcessorFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Processor {
	cfg := f.Wrapped.CreateDefaultConfig().(*resourceprocessor.Config)
	for k, v := range getTags(f.Viper) {
		cfg.Labels[k] = v
	}
	return cfg
}

func getTags(v *viper.Viper) map[string]string {
	tagsLegacy := flags.ParseJaegerTags(v.GetString(resourceTagsLegacy))
	tags := flags.ParseJaegerTags(v.GetString(resourceTags))
	for k, v := range tagsLegacy {
		if _, ok := tags[k]; !ok {
			tags[k] = v
		}
	}
	return tags
}

// CreateTraceProcessor creates resource processor.
// This function implements OTEL component.ProcessorFactoryOld interface.
func (f Factory) CreateTraceProcessor(
	logger *zap.Logger,
	nextConsumer consumer.TraceConsumerOld,
	cfg configmodels.Processor,
) (component.TraceProcessorOld, error) {
	return f.Wrapped.CreateTraceProcessor(logger, nextConsumer, cfg)
}

// CreateMetricsProcessor creates a resource processor.
// This function implements component.ProcessorFactoryOld.
func (f Factory) CreateMetricsProcessor(
	logger *zap.Logger,
	nextConsumer consumer.MetricsConsumerOld,
	cfg configmodels.Processor,
) (component.MetricsProcessorOld, error) {
	return f.Wrapped.CreateMetricsProcessor(logger, nextConsumer, cfg)
}

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.String(resourceTagsLegacy, "", "(deprecated, use --resource.tags) One or more tags to be added to the Process tags of all spans passing through this agent. Ex: key1=value1,key2=${envVar:defaultValue}")
	flags.String(resourceTags, "", "One or more tags to be added to the Process tags of all spans passing through this agent. Ex: key1=value1,key2=${envVar:defaultValue}")
}
