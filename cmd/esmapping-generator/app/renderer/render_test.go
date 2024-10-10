// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
)

func TestIsValidOption(t *testing.T) {
	tests := []struct {
		name          string
		arg           string
		expectedValue bool
	}{
		{name: "span mapping", arg: "jaeger-span", expectedValue: true},
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
		name          string
		args          app.Options
		wantedMapping *mappings.MappingBuilder
		want          string
		wantErr       error
	}{
		{
			name: "ES version 7",
			args: app.Options{
				Mapping:                "jaeger-span",
				EsVersion:              7,
				Shards:                 5,
				Replicas:               1,
				IndexPrefix:            "test-",
				UseILM:                 "true",
				ILMPolicyName:          "jaeger-test-policy",
				DisableLogsFieldSearch: "true",
			},
			wantedMapping: &mappings.MappingBuilder{
				EsVersion:              7,
				Shards:                 5,
				Replicas:               1,
				IndexPrefix:            "test-",
				UseILM:                 true,
				ILMPolicyName:          "jaeger-test-policy",
				DisableLogsFieldSearch: true,
			},
			want: "ES version 7",
		},
		{
			name: "Parse Error version 7",
			args: app.Options{
				Mapping:                "jaeger-span",
				EsVersion:              7,
				Shards:                 5,
				Replicas:               1,
				IndexPrefix:            "test-",
				UseILM:                 "true",
				ILMPolicyName:          "jaeger-test-policy",
				DisableLogsFieldSearch: "false",
			},
			wantedMapping: &mappings.MappingBuilder{
				EsVersion:              7,
				Shards:                 5,
				Replicas:               1,
				IndexPrefix:            "test-",
				UseILM:                 false,
				ILMPolicyName:          "jaeger-test-policy",
				DisableLogsFieldSearch: false,
			},
			wantErr: errors.New("parse error"),
		},
		{
			name: "Parse bool error for UseILM",
			args: app.Options{
				Mapping:       "jaeger-span",
				EsVersion:     7,
				Shards:        5,
				Replicas:      1,
				IndexPrefix:   "test-",
				UseILM:        "foo",
				ILMPolicyName: "jaeger-test-policy",
			},
			wantedMapping: &mappings.MappingBuilder{},
			wantErr:       errors.New("strconv.ParseBool: parsing \"foo\": invalid syntax"),
		},
		{
			name: "Parse bool error for DisableLogsFieldSearch",
			args: app.Options{
				Mapping:                "jaeger-span",
				EsVersion:              7,
				Shards:                 5,
				Replicas:               1,
				IndexPrefix:            "test-",
				UseILM:                 "false",
				ILMPolicyName:          "jaeger-test-policy",
				DisableLogsFieldSearch: "foo",
			},
			wantedMapping: &mappings.MappingBuilder{},
			wantErr:       errors.New("strconv.ParseBool: parsing \"foo\": invalid syntax"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			mockTemplateApplier := &mocks.TemplateApplier{}
			mockTemplateApplier.On("Execute", mock.Anything, mock.Anything).Return(
				func(wr io.Writer, data any) error {
					if actualMapping, ok := data.(*mappings.MappingBuilder); ok {
						require.Equal(t, tt.wantedMapping, actualMapping)
					} else {
						require.Fail(t, "unexpected mapping type")
					}
					wr.Write([]byte(tt.want))
					return nil
				},
			)
			mockTemplateBuilder := &mocks.TemplateBuilder{}
			tt.wantedMapping.TemplateBuilder = mockTemplateBuilder
			mockTemplateBuilder.On("Parse", mock.Anything).Return(mockTemplateApplier, tt.wantErr)

			// Test
			got, err := GetMappingAsString(mockTemplateBuilder, &tt.args)

			// Validate
			if tt.wantErr != nil {
				require.EqualError(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
