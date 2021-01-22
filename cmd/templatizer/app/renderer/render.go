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

	"github.com/jaegertracing/jaeger/cmd/templatizer/app"
	esTemplate "github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
)

var fixMappingFunc = es.FixMapping
var loadMappingFunc = es.LoadMapping
var mapping string
var err error

func GetMappingAsString(opt *app.Options) (string, error) {

	if opt.EsVersion == 7 {
		enableILM, err := strconv.ParseBool(opt.UseILM)
		if err != nil {
			return "", err
		}
		mapping, err = fixMappingFunc(esTemplate.TextTemplateBuilder{}, loadMappingFunc("/"+opt.Mapping+"-7.json"), opt.Shards, opt.Replicas, opt.EsPrefix, enableILM)
		if err != nil {
			return "", err
		}
	} else {
		mapping, err = fixMappingFunc(esTemplate.TextTemplateBuilder{}, loadMappingFunc("/"+opt.Mapping+".json"), opt.Shards, opt.Replicas, "", false)
		if err != nil {
			return "", err
		}
	}
	return mapping, nil

}

func IsValidOption(val string) bool {
	allowedValues := []string{"jaeger-span", "jaeger-service"}
	for _, value := range allowedValues {
		if val == value {
			return true
		}
	}
	return false
}
