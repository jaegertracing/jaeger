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

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

//go:embed fixtures/*.json
var FIXTURES embed.FS

func TestMappingBuilderGetMapping(t *testing.T) {
	const (
		jaegerSpan         = "jaeger-span"
		jaegerService      = "jaeger-service"
		jaegerDependencies = "jaeger-dependencies"
	)
	tests := []struct {
		mapping   string
		esVersion uint
	}{
		{mapping: jaegerSpan, esVersion: 8},
		{mapping: jaegerSpan, esVersion: 7},
		{mapping: jaegerSpan, esVersion: 6},
		{mapping: jaegerService, esVersion: 8},
		{mapping: jaegerService, esVersion: 7},
		{mapping: jaegerService, esVersion: 6},
		{mapping: jaegerDependencies, esVersion: 8},
		{mapping: jaegerDependencies, esVersion: 7},
		{mapping: jaegerDependencies, esVersion: 6},
	}
	for _, tt := range tests {
		t.Run(tt.mapping, func(t *testing.T) {
			defaultOpts := func(p int64) config.IndexOptions {
				return config.IndexOptions{
					Shards:   3,
					Replicas: 3,
					Priority: p,
				}
			}
			serviceOps := defaultOpts(501)
			dependenciesOps := defaultOpts(502)
			samplingOps := defaultOpts(503)

			mb := &MappingBuilder{
				TemplateBuilder: es.TextTemplateBuilder{},
				Indices: config.Indices{
					IndexPrefix:  "test-",
					Spans:        defaultOpts(500),
					Services:     serviceOps,
					Dependencies: dependenciesOps,
					Sampling:     samplingOps,
				},
				EsVersion:     tt.esVersion,
				UseILM:        true,
				ILMPolicyName: "jaeger-test-policy",
			}
			got, err := mb.GetMapping(tt.mapping)
			require.NoError(t, err)
			var wantbytes []byte
			fileSuffix := fmt.Sprintf("-%d", tt.esVersion)
			wantbytes, err = FIXTURES.ReadFile("fixtures/" + tt.mapping + fileSuffix + ".json")
			require.NoError(t, err)
			want := string(wantbytes)
			assert.Equal(t, want, got)
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
				Replicas: 5,
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
				EsVersion:     7,
				UseILM:        true,
				ILMPolicyName: "jaeger-test-policy",
			}
			_, err := mappingBuilder.renderMapping("test", mappingBuilder.getMappingTemplateOptions("test"))
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
		esVersion     uint
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
				esVersion:     7,
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
				esVersion:     7,
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
				esVersion:     6,
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
				esVersion:     6,
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
				esVersion:     6,
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
				esVersion:     7,
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
				Replicas: 3,
			}

			mappingBuilder := MappingBuilder{
				TemplateBuilder: test.mockNewTextTemplateBuilder(),
				Indices: config.Indices{
					Spans:        indexTemOps,
					Services:     indexTemOps,
					Dependencies: indexTemOps,
					Sampling:     indexTemOps,
				},
				EsVersion:     test.args.esVersion,
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
	}
	_, err := mappingBuilder.GetSamplingMappings()
	require.EqualError(t, err, "template load error")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
