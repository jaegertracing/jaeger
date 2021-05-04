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

package renderer

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
)

func TestIsValidOption(t *testing.T) {
	tests := []struct {
		name          string
		arg           string
		expectedValue bool
	}{{name: "span mapping", arg: "jaeger-span", expectedValue: true},
		{name: "service mapping", arg: "jaeger-service", expectedValue: true},
		{name: "Invalid mapping", arg: "dependency-service", expectedValue: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedValue, IsValidOption(test.arg))
		})
	}
}

func Test_getMappingAsString(t *testing.T) {
	tests := []struct {
		name    string
		args    app.Options
		want    string
		wantErr error
	}{
		{
			name: "ES version 7", args: app.Options{Mapping: "jaeger-span", EsVersion: 7, Shards: 5, Replicas: 1, IndexPrefix: "test", UseILM: "true", ILMPolicyName: "jaeger-test-policy"},
			want: "ES version 7",
		},
		{
			name: "ES version 6", args: app.Options{Mapping: "jaeger-span", EsVersion: 6, Shards: 5, Replicas: 1, IndexPrefix: "test", UseILM: "false", ILMPolicyName: "jaeger-test-policy"},
			want: "ES version 6",
		},
		{
			name: "Parse Error version 6", args: app.Options{Mapping: "jaeger-span", EsVersion: 6, Shards: 5, Replicas: 1, IndexPrefix: "test", UseILM: "false", ILMPolicyName: "jaeger-test-policy"},
			wantErr: errors.New("parse error"),
		},
		{
			name: "Parse Error version 7", args: app.Options{Mapping: "jaeger-span", EsVersion: 7, Shards: 5, Replicas: 1, IndexPrefix: "test", UseILM: "true", ILMPolicyName: "jaeger-test-policy"},
			wantErr: errors.New("parse error"),
		},
		{
			name: "Parse bool error", args: app.Options{Mapping: "jaeger-span", EsVersion: 7, Shards: 5, Replicas: 1, IndexPrefix: "test", UseILM: "foo", ILMPolicyName: "jaeger-test-policy"},
			wantErr: errors.New("strconv.ParseBool: parsing \"foo\": invalid syntax"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			mockTemplateApplier := &mocks.TemplateApplier{}
			mockTemplateApplier.On("Execute", mock.Anything, mock.Anything).Return(
				func(wr io.Writer, data interface{}) error {
					wr.Write([]byte(tt.want))
					return nil
				},
			)
			mockTemplateBuilder := &mocks.TemplateBuilder{}
			mockTemplateBuilder.On("Parse", mock.Anything).Return(mockTemplateApplier, tt.wantErr)

			// Test
			got, err := GetMappingAsString(mockTemplateBuilder, &tt.args)

			// Validate
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
