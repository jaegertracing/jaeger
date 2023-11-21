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
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
)

//go:embed fixtures/*.json
var FIXTURES embed.FS

func TestMappingBuilder_GetMapping(t *testing.T) {
	tests := []struct {
		esVersion     uint
		useILM        bool
		mappingType   string
		fixtureName   string
		logsFieldType FieldType
	}{
		{mappingType: "span", esVersion: 7, useILM: true, fixtureName: "jaeger-span-with-ilm-7"},
		{mappingType: "span", esVersion: 7, logsFieldType: ObjectFieldType, fixtureName: "jaeger-span-with-object-fieldtype-logs-7"},
		{mappingType: "span", esVersion: 7, useILM: false, fixtureName: "jaeger-span-7"},
		{mappingType: "span", esVersion: 6, fixtureName: "jaeger-span"},
		{mappingType: "span", esVersion: 6, logsFieldType: ObjectFieldType, fixtureName: "jaeger-span-with-object-fieldtype-logs"},
		{mappingType: "service", esVersion: 7, useILM: true, fixtureName: "jaeger-service-with-ilm-7"},
		{mappingType: "service", esVersion: 7, useILM: false, fixtureName: "jaeger-service-7"},
		{mappingType: "service", esVersion: 6, fixtureName: "jaeger-service"},
		{mappingType: "dependencies", esVersion: 7, useILM: true, fixtureName: "jaeger-dependencies-with-ilm-7"},
		{mappingType: "dependencies", esVersion: 7, useILM: false, fixtureName: "jaeger-dependencies-7"},
		{mappingType: "dependencies", esVersion: 6, fixtureName: "jaeger-dependencies"},
	}
	for _, tt := range tests {
		mapping := fmt.Sprintf("jaeger-%s", tt.mappingType)
		testName := fmt.Sprintf("%s-%v-ilm-%v-log-field-type-%v", mapping, tt.esVersion, tt.useILM, tt.logsFieldType)
		t.Run(testName, func(t *testing.T) {
			mb := &MappingBuilder{
				TemplateBuilder: es.TextTemplateBuilder{},
				Shards:          3,
				Replicas:        3,
				EsVersion:       tt.esVersion,
				IndexPrefix:     "test-",
				UseILM:          tt.useILM,
				ILMPolicyName:   "jaeger-test-policy",
				LogsFieldsType:  tt.logsFieldType,
			}

			got, err := mb.GetMapping(mapping)
			require.NoError(t, err)

			var wantBytes []byte
			wantBytes, err = FIXTURES.ReadFile(fmt.Sprintf("fixtures/%s.json", tt.fixtureName))
			want := string(wantBytes)
			assert.NoError(t, err)
			assert.Equal(t, got, want)
		})
	}
}

func TestMappingBuilder_loadMapping(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "jaeger-span.json"},
		{name: "jaeger-service.json"},
		{name: "jaeger-span-7.json"},
		{name: "jaeger-service-7.json"},
		{name: "jaeger-dependencies.json"},
		{name: "jaeger-dependencies-7.json"},
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
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
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
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
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
				ILMPolicyName:   "jaeger-test-policy",
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
		shards        int64
		replicas      int64
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
				shards:        3,
				replicas:      3,
				esVersion:     7,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(nil)
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
				return &tb
			},
			err: "",
		},
		{
			name: "ES Version 7 Service Error",
			args: args{
				shards:        3,
				replicas:      3,
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
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
				return &tb
			},
			err: "template load error",
		},

		{
			name: "ES Version < 7",
			args: args{
				shards:        3,
				replicas:      3,
				esVersion:     6,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(nil)
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
				return &tb
			},
			err: "",
		},
		{
			name: "ES Version < 7 Service Error",
			args: args{
				shards:        3,
				replicas:      3,
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
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				return &tb
			},
			err: "template load error",
		},
		{
			name: "ES Version < 7 Span Error",
			args: args{
				shards:        3,
				replicas:      3,
				esVersion:     6,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error"))
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
				return &tb
			},
			err: "template load error",
		},
		{
			name: "ES Version  7 Span Error",
			args: args{
				shards:        3,
				replicas:      3,
				esVersion:     7,
				indexPrefix:   "test",
				useILM:        true,
				ilmPolicyName: "jaeger-test-policy",
			},
			mockNewTextTemplateBuilder: func() es.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything, mock.Anything).Return(errors.New("template load error")).Once()
				tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
				tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)
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
				ILMPolicyName:   test.args.ilmPolicyName,
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
	tb.On("ParseFieldType", mock.Anything).Return(&ta, nil)
	tb.On("Parse", mock.Anything).Times(2).Return(&ta, nil)

	mappingBuilder := MappingBuilder{
		TemplateBuilder: &tb,
	}
	_, err := mappingBuilder.GetDependenciesMappings()
	assert.EqualError(t, err, "template load error")
}
