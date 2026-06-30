// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap/zaptest"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
)

func newDataStreamFactory(t *testing.T, version es.BackendVersion, ds escfg.DataStreamRotation) (*FactoryBase, *mocks.Client) {
	mockClient := &mocks.Client{}
	mockClient.On("GetVersion").Return(version)
	f := &FactoryBase{
		client:          mockClient,
		logger:          zaptest.NewLogger(t),
		templateBuilder: es.TextTemplateBuilder{},
		config: &escfg.Configuration{
			CreateIndexTemplates: true,
			Indices: escfg.Indices{
				Spans: escfg.IndexOptions{
					Shards:   3,
					Replicas: new(int64(1)),
					Rotation: escfg.RotationConfig{DataStream: configoptional.Some(ds)},
				},
			},
		},
	}
	return f, mockClient
}

func (f *FactoryBase) runBootstrap() error {
	mb := f.mappingBuilderFromConfig(f.config)
	return f.bootstrapSpanDataStream(context.Background(), &mb)
}

func TestBootstrapSpanDataStream(t *testing.T) {
	tests := []struct {
		name       string
		version    es.BackendVersion
		ds         escfg.DataStreamRotation
		wantPolicy string
	}{
		{"elasticsearch default policy", es.ElasticV8, escfg.DataStreamRotation{}, defaultDataStreamPolicyName},
		{"opensearch default policy", es.OpenSearch2, escfg.DataStreamRotation{}, defaultDataStreamPolicyName},
		{"custom policy name", es.ElasticV8, escfg.DataStreamRotation{PolicyName: "custom-policy"}, "custom-policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, mc := newDataStreamFactory(t, tt.version, tt.ds)
			mc.On("LifecyclePolicyExists", mock.Anything, tt.wantPolicy).Return(false, nil)
			mc.On("CreateLifecyclePolicy", mock.Anything, tt.wantPolicy, mock.Anything).Return(nil)
			mc.On("CreateComponentTemplate", mock.Anything, "jaeger.spans@mappings", mock.Anything).Return(nil)
			mc.On("CreateComponentTemplate", mock.Anything, "jaeger.spans@settings", mock.Anything).Return(nil)
			mc.On("CreateComposableIndexTemplate", mock.Anything, "jaeger.spans", mock.Anything).Return(nil)

			require.NoError(t, f.runBootstrap())
			mc.AssertExpectations(t)
		})
	}
}

func TestBootstrapSpanDataStream_PolicyAlreadyExists(t *testing.T) {
	f, mc := newDataStreamFactory(t, es.ElasticV8, escfg.DataStreamRotation{})
	mc.On("LifecyclePolicyExists", mock.Anything, defaultDataStreamPolicyName).Return(true, nil)
	mc.On("CreateComponentTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mc.On("CreateComposableIndexTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, f.runBootstrap())
	// An existing policy must never be overwritten (RFC 0004 §3.6).
	mc.AssertNotCalled(t, "CreateLifecyclePolicy", mock.Anything, mock.Anything, mock.Anything)
}

func TestBootstrapSpanDataStream_PolicyFileOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"policy":{"custom":true}}`), 0o600))

	f, mc := newDataStreamFactory(t, es.ElasticV8, escfg.DataStreamRotation{PolicyFile: path})
	mc.On("LifecyclePolicyExists", mock.Anything, defaultDataStreamPolicyName).Return(false, nil)
	mc.On("CreateLifecyclePolicy", mock.Anything, defaultDataStreamPolicyName, `{"policy":{"custom":true}}`).Return(nil)
	mc.On("CreateComponentTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mc.On("CreateComposableIndexTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, f.runBootstrap())
	mc.AssertExpectations(t)
}

func TestBootstrapSpanDataStream_Errors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		ds     escfg.DataStreamRotation
		setup  func(mc *mocks.Client)
		errMsg string
	}{
		{
			name: "lifecycle exists check fails",
			setup: func(mc *mocks.Client) {
				mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(false, boom)
			},
			errMsg: "failed to check lifecycle policy",
		},
		{
			name: "lifecycle create fails",
			setup: func(mc *mocks.Client) {
				mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(false, nil)
				mc.On("CreateLifecyclePolicy", mock.Anything, mock.Anything, mock.Anything).Return(boom)
			},
			errMsg: "failed to create lifecycle policy",
		},
		{
			name: "policy file missing",
			ds:   escfg.DataStreamRotation{PolicyFile: filepath.Join(t.TempDir(), "does-not-exist.json")},
			setup: func(mc *mocks.Client) {
				mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(false, nil)
			},
			errMsg: "failed to read data stream policy file",
		},
		{
			name: "mappings component template fails",
			setup: func(mc *mocks.Client) {
				mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(true, nil)
				mc.On("CreateComponentTemplate", mock.Anything, "jaeger.spans@mappings", mock.Anything).Return(boom)
			},
			errMsg: "boom",
		},
		{
			name: "settings component template fails",
			setup: func(mc *mocks.Client) {
				mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(true, nil)
				mc.On("CreateComponentTemplate", mock.Anything, "jaeger.spans@mappings", mock.Anything).Return(nil)
				mc.On("CreateComponentTemplate", mock.Anything, "jaeger.spans@settings", mock.Anything).Return(boom)
			},
			errMsg: "boom",
		},
		{
			name: "index template fails",
			setup: func(mc *mocks.Client) {
				mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(true, nil)
				mc.On("CreateComponentTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mc.On("CreateComposableIndexTemplate", mock.Anything, mock.Anything, mock.Anything).Return(boom)
			},
			errMsg: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, mc := newDataStreamFactory(t, es.ElasticV8, tt.ds)
			tt.setup(mc)
			require.ErrorContains(t, f.runBootstrap(), tt.errMsg)
		})
	}
}

func TestBootstrapSpanDataStream_MappingBuildError(t *testing.T) {
	f, mc := newDataStreamFactory(t, es.ElasticV8, escfg.DataStreamRotation{})
	mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(true, nil)
	tb := &mocks.TemplateBuilder{}
	tb.On("Parse", mock.Anything).Return(nil, errors.New("parse boom"))
	mb := &mappings.MappingBuilder{
		TemplateBuilder: tb,
		Indices:         f.config.Indices,
		Version:         es.ElasticV8,
	}
	require.ErrorContains(t, f.bootstrapSpanDataStream(context.Background(), mb), "failed to build data stream mappings")
}

// TestCreateTemplates_DataStreamRouting verifies createTemplates routes spans to
// the data stream bootstrap while still creating the legacy service template.
func TestCreateTemplates_DataStreamRouting(t *testing.T) {
	f, mc := newDataStreamFactory(t, es.ElasticV8, escfg.DataStreamRotation{})
	f.config.Indices.Services = escfg.IndexOptions{Shards: 3, Replicas: new(int64(1))}
	mc.On("LifecyclePolicyExists", mock.Anything, mock.Anything).Return(true, nil)
	mc.On("CreateComponentTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mc.On("CreateComposableIndexTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	svc := &mocks.TemplateCreateService{}
	svc.On("Body", mock.Anything).Return(svc)
	svc.On("Do", mock.Anything).Return(nil, nil)
	mc.On("CreateTemplate", escfg.IndexPrefix("").Apply(escfg.ServiceIndexName)).Return(svc)

	require.NoError(t, f.createTemplates(context.Background()))
	// Spans never used the legacy _template path.
	mc.AssertNotCalled(t, "CreateTemplate", escfg.IndexPrefix("").Apply(escfg.SpanIndexName))
	mc.AssertExpectations(t)
}
