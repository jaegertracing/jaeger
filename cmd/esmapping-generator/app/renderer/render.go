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
	"strconv"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
)

var supportedMappings = map[string]struct{}{
	"jaeger-span":         {},
	"jaeger-service":      {},
	"jaeger-dependencies": {},
}

// GetMappingAsString returns rendered index templates as string
func GetMappingAsString(builder es.TemplateBuilder, opt *app.Options) (string, error) {

	enableILM, err := strconv.ParseBool(opt.UseILM)
	if err != nil {
		return "", err
	}

	mappingBuilder := mappings.MappingBuilder{
		TemplateBuilder: builder,
		Shards:          opt.Shards,
		Replicas:        opt.Replicas,
		EsVersion:       opt.EsVersion,
		IndexPrefix:     opt.IndexPrefix,
		UseILM:          enableILM,
		ILMPolicyName:   opt.ILMPolicyName,
	}
	return mappingBuilder.GetMapping(opt.Mapping)
}

// IsValidOption checks if passed option is a valid index template.
func IsValidOption(val string) bool {
	_, ok := supportedMappings[val]
	return ok
}
