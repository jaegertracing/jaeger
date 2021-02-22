// Copyright (c) 2021 The Jaeger Authors.
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

package mappings

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
)

func TestMappingBuilder_GetMapping(t *testing.T) {
	tests := []struct {
		mapping   string
		esVersion uint
	}{
		{mapping: "jaeger-span", esVersion: 7},
		{mapping: "jaeger-span", esVersion: 6},
		{mapping: "jaeger-service", esVersion: 7},
		{mapping: "jaeger-service", esVersion: 6},
		{mapping: "jaeger-dependencies", esVersion: 7},
		{mapping: "jaeger-dependencies", esVersion: 6},
	}
	for _, tt := range tests {
		t.Run(tt.mapping, func(t *testing.T) {
			mb := &MappingBuilder{
				TemplateBuilder: es.TextTemplateBuilder{},
				Shards:          3,
				Replicas:        3,
				EsVersion:       tt.esVersion,
				IndexPrefix:     "",
				UseILM:          false,
			}
			got, err := mb.GetMapping(tt.mapping)
			require.NoError(t, err)
			want := ""
			if tt.esVersion == 7 {
				want, err = mb.fixMapping("/" + tt.mapping + "-7.json")
				require.NoError(t, err)
			} else {
				want, err = mb.fixMapping("/" + tt.mapping + ".json")
				require.NoError(t, err)
			}
			assert.Equal(t, got, want)
		})
	}
}

func TestMappingBuilder_loadMapping(t *testing.T) {
	tests := []struct {
		name        string
		indexPrefix string
		useILM      bool
	}{
		{name: "/jaeger-span.json"},
		{name: "/jaeger-service.json"},
		{name: "/jaeger-span-7.json"},
		{name: "/jaeger-service-7.json"},
		{name: "/jaeger-dependencies.json"},
		{name: "/jaeger-dependencies-7.json"},
	}
	for _, test := range tests {
		mapping := loadMapping(test.name)
		f, err := os.Open("./" + test.name)
		require.NoError(t, err)
		b, err := ioutil.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, string(b), mapping)
		_, err = template.New("mapping").Parse(mapping)
		require.NoError(t, err)
	}
}

func TestMappingBuilder_fixMapping(t *testing.T) {
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
			mappingBuilder := MappingBuilder{
				TemplateBuilder: test.templateBuilderMockFunc(),
				Shards:          3,
				Replicas:        5,
				EsVersion:       7,
				IndexPrefix:     "test",
				UseILM:          true,
			}
			_, err := mappingBuilder.fixMapping("test")
			if test.err != "" {
				assert.EqualError(t, err, test.err)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}

func TestMappingBuilder_GetSpanServiceMappings(t *testing.T) {
	type args struct {
		shards      int64
		replicas    int64
		esVersion   uint
		indexPrefix string
		useILM      bool
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
				shards:      3,
				replicas:    3,
				esVersion:   7,
				indexPrefix: "test",
				useILM:      true,
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
				shards:      3,
				replicas:    3,
				esVersion:   7,
				indexPrefix: "test",
				useILM:      true,
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
				shards:      3,
				replicas:    3,
				esVersion:   6,
				indexPrefix: "test",
				useILM:      true,
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
				shards:      3,
				replicas:    3,
				esVersion:   6,
				indexPrefix: "test",
				useILM:      true,
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
				shards:      3,
				replicas:    3,
				esVersion:   6,
				indexPrefix: "test",
				useILM:      true,
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
				shards:      3,
				replicas:    3,
				esVersion:   7,
				indexPrefix: "test",
				useILM:      true,
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
			mappingBuilder := MappingBuilder{
				TemplateBuilder: test.mockNewTextTemplateBuilder(),
				Shards:          test.args.shards,
				Replicas:        test.args.replicas,
				EsVersion:       test.args.esVersion,
				IndexPrefix:     test.args.indexPrefix,
				UseILM:          test.args.useILM,
			}
			_, _, err := mappingBuilder.GetSpanServiceMappings()
			if test.err != "" {
				assert.EqualError(t, err, test.err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMappingBuilder_GetDependenciesMappings(t *testing.T) {
	tb := mocks.TemplateBuilder{}
	ta := mocks.TemplateApplier{}
	ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error"))
	tb.On("Parse", mock.Anything).Return(&ta, nil)

	mappingBuilder := MappingBuilder{
		TemplateBuilder: &tb,
		Shards:          5,
		Replicas:        5,
		EsVersion:       7,
		IndexPrefix:     "",
		UseILM:          false,
	}
	_, err := mappingBuilder.GetDependenciesMappings()
	assert.EqualError(t, err, "template load error")
}
