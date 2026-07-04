// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

//go:embed fixtures/*.json
var FIXTURES embed.FS

// allMappingTypes and the version lists below define the full
// {mapping type × backend × version} matrix that the golden tests assert.
var (
	allMappingTypes    = []MappingType{SpanMapping, ServiceMapping, DependenciesMapping, SamplingMapping}
	elasticVersions    = []es.BackendVersion{es.ElasticV6, es.ElasticV7, es.ElasticV8, es.ElasticV9}
	openSearchVersions = []es.BackendVersion{es.OpenSearch1, es.OpenSearch2, es.OpenSearch3}
)

// fixtureName returns the golden fixture file name for a given mapping type and
// backend version. ES8/ES9 both render the "-8" template; all OpenSearch
// versions render the "-7-opensearch" template.
func fixtureName(mapping MappingType, version es.BackendVersion) string {
	suffix := fmt.Sprintf("-%d", version.TemplateVersion())
	if version.IsOpenSearch() {
		suffix += "-opensearch"
	}
	return mapping.String() + suffix + ".json"
}

func newTestMappingBuilder(version es.BackendVersion) *MappingBuilder {
	defaultOpts := func(p int64) config.IndexOptions {
		return config.IndexOptions{
			Shards:   3,
			Replicas: new(int64(3)),
			Priority: p,
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

func assertGoldenMatrix(t *testing.T, versions []es.BackendVersion) {
	for _, version := range versions {
		for _, mapping := range allMappingTypes {
			t.Run(fmt.Sprintf("%s/%s", mapping.String(), version), func(t *testing.T) {
				mb := newTestMappingBuilder(version)
				got, err := mb.GetMapping(mapping)
				require.NoError(t, err)
				wantbytes, err := FIXTURES.ReadFile("fixtures/" + fixtureName(mapping, version))
				require.NoError(t, err)
				assert.Equal(t, string(wantbytes), got)
			})
		}
	}
}

func TestMappingBuilderGetMapping(t *testing.T) {
	assertGoldenMatrix(t, elasticVersions)
}

func TestMappingBuilderGetMapping_OpenSearch(t *testing.T) {
	assertGoldenMatrix(t, openSearchVersions)
}

// TestGoldenFixturesAreAllUsed guards against orphaned fixtures: every committed
// golden file must be loaded by some {mapping type × backend × version} cell in
// the matrix above. A fixture that no test ever reads (e.g. the dead
// "-8-opensearch" files) fails this test.
func TestGoldenFixturesAreAllUsed(t *testing.T) {
	used := make(map[string]bool)
	for _, versions := range [][]es.BackendVersion{elasticVersions, openSearchVersions} {
		for _, version := range versions {
			for _, mapping := range allMappingTypes {
				used[fixtureName(mapping, version)] = true
			}
		}
	}
	entries, err := FIXTURES.ReadDir("fixtures")
	require.NoError(t, err)
	for _, entry := range entries {
		assert.Truef(t, used[entry.Name()],
			"fixture %q is committed but never loaded by any golden test", entry.Name())
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
