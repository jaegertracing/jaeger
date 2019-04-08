// Copyright (c) 2019 The Jaeger Authors.
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
	"errors"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.Factory = new(Factory)

type mockPluginBuilder struct {
	plugin *mockPlugin
	err error
}

func (b *mockPluginBuilder) Build() (shared.StoragePlugin, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.plugin, nil
}

type mockPlugin struct {

}

func (*mockPlugin) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	panic("implement me")
}

func (*mockPlugin) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	panic("implement me")
}

func (*mockPlugin) GetServices(ctx context.Context) ([]string, error) {
	panic("implement me")
}

func (*mockPlugin) GetOperations(ctx context.Context, service string) ([]string, error) {
	panic("implement me")
}

func (*mockPlugin) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	panic("implement me")
}

func (*mockPlugin) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	panic("implement me")
}

func (*mockPlugin) WriteSpan(span *model.Span) error {
	panic("implement me")
}

func TestGRPCStorageFactory(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v)

	// after InitFromViper, f.builder points to a real plugin builder that will fail in unit tests,
	// so we override it with a mock.
	f.builder = &mockPluginBuilder{
		err: errors.New("made-up error"),
	}
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{},
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	assert.NotNil(t, f.store)
	reader, err := f.CreateSpanReader()
	assert.NoError(t, err)
	assert.Equal(t, f.store, reader)
	writer, err := f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store, writer)
	depReader, err := f.CreateDependencyReader()
	assert.NoError(t, err)
	assert.Equal(t, f.store, depReader)
}

func TestWithConfiguration(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{
		"--grpc-storage-plugin.binary=noop-grpc-plugin",
		"--grpc-storage-plugin.configuration-file=config.json",
	})
	f.InitFromViper(v)
	assert.Equal(t, f.options.Configuration.PluginBinary, "noop-grpc-plugin")
	assert.Equal(t, f.options.Configuration.PluginConfigurationFile, "config.json")
}
