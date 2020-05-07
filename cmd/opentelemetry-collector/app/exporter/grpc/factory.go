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

package grpc

import (
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"

	storageGrpc "github.com/jaegertracing/jaeger/plugin/storage/grpc"
)

const (
	TypeStr = "jaeger_grpc"
)

// OptionsFactory returns initialized es.OptionsFactory structure.
type OptionsFactory func() *storageGrpc.Options

// DefaultOptions creates gRPC options supported by this exporter.
func DefaultOptions() *storageGrpc.Options {
	return &storageGrpc.Options{}
}

// Factory is the factory for Jaeger gRPC exporter.
type Factory struct {
	OptionsFactory OptionsFactory
}

var _ component.ExporterFactory = (*Factory)(nil)

// Type returns the type of exporter.
func (f Factory) Type() configmodels.Type {
	return TypeStr
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ExporterFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Exporter {
	opts := f.OptionsFactory()
	return &Config{
		Options: *opts,
		ExporterSettings: configmodels.ExporterSettings{
			TypeVal: TypeStr,
			NameVal: TypeStr,
		},
	}
}

// CreateTraceExporter creates Jaeger gRPC trace exporter.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateTraceExporter(
	_ context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TraceExporter, error) {
	grpcCfg, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	return new(grpcCfg, params)
}

// CreateMetricsExporter is not implemented.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateMetricsExporter(
	_ context.Context,
	_ component.ExporterCreateParams,
	_ configmodels.Exporter,
) (component.MetricsExporter, error) {
	return nil, configerror.ErrDataTypeIsNotSupported
}
