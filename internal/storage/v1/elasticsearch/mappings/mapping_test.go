// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

// TestMappingBuilderGetMapping snapshots the rendered index template for every
// mapping type across the full ES 6/7/8/9 + OpenSearch 1/2/3 matrix, using the
// §7.3 fixture taxonomy (testdata/<subject>.<backend><range>.json).
// Byte-identical consecutive majors collapse into a range, so the fixture tree
// itself is the compatibility matrix.
func TestMappingBuilderGetMapping(t *testing.T) {
	subjects := []struct {
		name    string
		mapping MappingType
	}{
		{"span", SpanMapping},
		{"service", ServiceMapping},
		{"dependencies", DependenciesMapping},
		{"sampling", SamplingMapping},
	}
	for _, subject := range subjects {
		t.Run(subject.name, func(t *testing.T) {
			content := map[es.BackendVersion]string{}
			for _, version := range snapshottest.AllVersions {
				got, err := newTestMappingBuilder(version).GetMapping(subject.mapping)
				require.NoError(t, err)
				content[version] = got
			}
			snapshottest.AssertVersionedGoldens(t, filepath.Join("testdata", subject.name), content)
		})
	}
}

func newTestMappingBuilder(version es.BackendVersion) *MappingBuilder {
	defaultOpts := func(priority int64) config.IndexOptions {
		return config.IndexOptions{
			Shards:   3,
			Replicas: new(int64(3)),
			Priority: priority,
		}
	}
	return &MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices: config.Indices{
			IndexPrefix:  "test-",
			Spans:        defaultOpts(500),
			Services:     defaultOpts(501),
			Dependencies: defaultOpts(502),
			Sampling:     defaultOpts(503),
		},
		Version:       version,
		UseILM:        true,
		ILMPolicyName: "jaeger-test-policy",
	}
}

func TestMappingTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected MappingType
		hasError bool
	}{
		{config.SpanIndexName, SpanMapping, false},
		{config.ServiceIndexName, ServiceMapping, false},
		{config.DependencyIndexName, DependenciesMapping, false},
		{config.SamplingIndexName, SamplingMapping, false},
		{"invalid", MappingType(-1), true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := MappingTypeFromString(tt.input)
			if tt.hasError {
				require.Error(t, err)
				assert.Equal(t, "unknown", result.String())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMappingBuilderLoadMapping(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "jaeger-span-6.json"},
		{name: "jaeger-span-7.json"},
		{name: "jaeger-span-8.json"},
		{name: "jaeger-service-6.json"},
		{name: "jaeger-service-7.json"},
		{name: "jaeger-service-8.json"},
		{name: "jaeger-dependencies-6.json"},
		{name: "jaeger-dependencies-7.json"},
		{name: "jaeger-dependencies-8.json"},
	}
	for _, test := range tests {
		mapping := loadMapping(test.name)
		f, err := os.Open("./" + test.name)
		require.NoError(t, err)
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, string(b), mapping)
		_, err = template.New("mapping").Parse(mapping)
		require.NoError(t, err)
	}
}

func TestMappingBuilderFixMapping(t *testing.T) {
	tests := []struct {
		name                    string
		templateBuilderMockFunc func() *mocks.TemplateBuilder
		err                     string
	}{
		{
			name: "templateRenderSuccess",
			templateBuilderMockFunc: func() *mocks.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(nil)
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "",
		},
		{
			name: "templateRenderFailure",
			templateBuilderMockFunc: func() *mocks.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template exec error"))
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "template exec error",
		},
		{
			name: "templateLoadError",
			templateBuilderMockFunc: func() *mocks.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				tb.On("Parse", mock.Anything).Return(nil, errors.New("template load error"))
				return &tb
			},
			err: "template load error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indexTemOps := config.IndexOptions{
				Shards:   3,
				Replicas: new(int64(5)),
				Priority: 500,
			}
			mappingBuilder := MappingBuilder{
				TemplateBuilder: test.templateBuilderMockFunc(),
				Indices: config.Indices{
					Spans:        indexTemOps,
					Services:     indexTemOps,
					Dependencies: indexTemOps,
					Sampling:     indexTemOps,
				},
				Version:       es.ElasticV7,
				UseILM:        true,
				ILMPolicyName: "jaeger-test-policy",
			}
			_, err := mappingBuilder.renderMapping("test", mappingBuilder.getMappingTemplateOptions(SpanMapping))
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMappingBuilderGetSpanServiceMappings(t *testing.T) {
	type args struct {
		version       es.BackendVersion
		indexPrefix   string
		useILM        bool
		ilmPolicyName string
	}
	tests := []struct {
		name                       string
		args                       args
		mockNewTextTemplateBuilder func() es.TemplateBuilder
		err                        string
	}{
		{
			name: "ES Version 7",
			args: args{
				version:       es.ElasticV7,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(nil)
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "",
		},
		{
			name: "ES Version 7 Service Error",
			args: args{
				version:       es.ElasticV7,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(nil).Once()
				ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error")).Once()
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "template load error",
		},

		{
			name: "ES Version < 7",
			args: args{
				version:       es.ElasticV6,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(nil)
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "",
		},
		{
			name: "ES Version < 7 Service Error",
			args: args{
				version:       es.ElasticV6,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(nil).Once()
				ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error")).Once()
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "template load error",
		},
		{
			name: "ES Version < 7 Span Error",
			args: args{
				version:       es.ElasticV6,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error"))
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "template load error",
		},
		{
			name: "ES Version  7 Span Error",
			args: args{
				version:       es.ElasticV7,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error")).Once()
				tb.On("Parse", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "template load error",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indexTemOps := config.IndexOptions{
				Shards:   3,
				Replicas: new(int64(3)),
			}

			mappingBuilder := MappingBuilder{
				TemplateBuilder: test.mockNewTextTemplateBuilder(),
				Indices: config.Indices{
					Spans:        indexTemOps,
					Services:     indexTemOps,
					Dependencies: indexTemOps,
					Sampling:     indexTemOps,
				},
				Version:       test.args.version,
				UseILM:        test.args.useILM,
				ILMPolicyName: test.args.ilmPolicyName,
			}
			_, _, err := mappingBuilder.GetSpanServiceMappings()
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMappingBuilderGetDependenciesMappings(t *testing.T) {
	tb := mocks.TemplateBuilder{}
	ta := mocks.TemplateApplier{}
	ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error"))
	tb.On("Parse", mock.Anything).Return(&ta, nil)

	mappingBuilder := MappingBuilder{
		TemplateBuilder: &tb,
		Indices: config.Indices{
			Dependencies: config.IndexOptions{
				Replicas: new(int64(1)),
				Shards:   3,
				Priority: 10,
			},
		},
	}
	_, err := mappingBuilder.GetDependenciesMappings()
	require.EqualError(t, err, "template load error")
}

func TestMappingBuilderGetSamplingMappings(t *testing.T) {
	tb := mocks.TemplateBuilder{}
	ta := mocks.TemplateApplier{}
	ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error"))
	tb.On("Parse", mock.Anything).Return(&ta, nil)

	mappingBuilder := MappingBuilder{
		TemplateBuilder: &tb,
		Indices: config.Indices{
			Sampling: config.IndexOptions{
				Replicas: new(int64(1)),
				Shards:   3,
				Priority: 10,
			},
		},
	}
	_, err := mappingBuilder.GetSamplingMappings()
	require.EqualError(t, err, "template load error")
}

func TestGetMappingTemplateOptions_DefaultCase(t *testing.T) {
	mappingBuilder := &MappingBuilder{
		Indices: config.Indices{
			Spans: config.IndexOptions{
				Shards:   2,
				Replicas: new(int64(1)),
				Priority: 10,
			},
		},
		UseILM:        true,
		ILMPolicyName: "test-policy",
	}

	opts := mappingBuilder.getMappingTemplateOptions(MappingType(-1))

	assert.Equal(t, int64(5), opts.Shards)
	assert.Equal(t, int64(1), opts.Replicas)
	assert.Equal(t, int64(0), opts.Priority)
	assert.True(t, opts.UseILM)
	assert.Equal(t, "test-policy", opts.ILMPolicyName)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
