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

package main

import (
	"errors"
	"testing"

	esTemplate "github.com/jaegertracing/jaeger/pkg/es"
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
			if got := isValidOption(test.arg); got != test.expectedValue {
				t.Errorf("isValidOption() = %v, want %v", got, test.expectedValue)
			}
		})
	}
}

func Test_getMappingAsString(t *testing.T) {
	type args struct {
		mappingName string
		esPrefix    string
		useILM      string
		esVersion   int64
		shards      int64
		replicas    int64
	}
	tests := []struct {
		name           string
		args           args
		fixMappingFunc func(esTemplate.TemplateBuilder, string, int64, int64, string, bool) (string, error)
		want           string
		wantErr        bool
	}{
		{name: "ES version 7", args: args{"jaeger-span", "test", "true", 7, 5, 5},
			fixMappingFunc: func(esTemplate.TemplateBuilder, string, int64, int64, string, bool) (string, error) {
				return "ES version 7", nil
			},
			want:    "ES version 7",
			wantErr: false,
		},
		{name: "ES version 6", args: args{"jaeger-service", "test", "false", 6, 5, 5},
			fixMappingFunc: func(esTemplate.TemplateBuilder, string, int64, int64, string, bool) (string, error) {
				return "ES version 6", nil
			},
			want:    "ES version 6",
			wantErr: false,
		},
		{name: "Parse Error version 6", args: args{"jaeger-service", "test", "false", 6, 5, 5},
			fixMappingFunc: func(esTemplate.TemplateBuilder, string, int64, int64, string, bool) (string, error) {
				return "", errors.New("parse error")
			},
			want:    "",
			wantErr: true,
		}, {name: "Parse Error version 7", args: args{"jaeger-service", "test", "false", 7, 5, 5},
			fixMappingFunc: func(esTemplate.TemplateBuilder, string, int64, int64, string, bool) (string, error) {
				return "", errors.New("parse error")
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			oldFixMappingFunc := fixMappingFunc
			oldLoadMappingFunc := loadMappingFunc
			defer func() {
				fixMappingFunc = oldFixMappingFunc
				loadMappingFunc = oldLoadMappingFunc
			}()

			fixMappingFunc = tt.fixMappingFunc
			loadMappingFunc = func(string) string { return "test" }
			got, err := getMappingAsString(tt.args.mappingName, tt.args.esPrefix, tt.args.useILM, tt.args.esVersion, tt.args.shards, tt.args.replicas)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMappingAsString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getMappingAsString() got = %v, want %v", got, tt.want)
			}
		})
	}
}
