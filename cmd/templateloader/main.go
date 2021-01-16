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
	"flag"
	"strconv"

	"go.uber.org/zap"

	esTemplate "github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
)

var logger, _ = zap.NewDevelopment()
var fixMappingFunc = es.FixMapping
var loadMappingFunc = es.LoadMapping
var mapping string
var err error

func main() {

	mappingName := flag.String("mapping", "", "Pass name of the mapping to parse - should be either jaeger-service or jaeger-span")
	esVersion := flag.Int64("esVersion", 7, "Version of ES")
	shards := flag.Int64("shards", 3, "Number of shards")
	replicas := flag.Int64("replicas", 3, "Number of replicas")
	esPrefix := flag.String("esPrefix", "", "Prefix for ES indices")
	useILM := flag.String("useILM", "false", "Set to true to enable ILM")
	flag.Parse()
	if !isValidOption(*mappingName) {
		logger.Fatal("please pass either 'jaeger-service' or 'jaeger-span' as argument")
	}
	parsedMapping, err := getMappingAsString(*mappingName, *esPrefix, *useILM, *esVersion, *shards, *replicas)
	if err != nil {
		logger.Fatal(err.Error())
	}
	print(parsedMapping)

}

func getMappingAsString(mappingName, esPrefix, useILM string, esVersion, shards, replicas int64) (string, error) {

	if esVersion == 7 {
		enableILM, err := strconv.ParseBool(useILM)
		if err != nil {
			return "", err
		}
		mapping, err = fixMappingFunc(esTemplate.TextTemplateBuilder{}, loadMappingFunc("/"+mappingName+"-7.json"), shards, replicas, esPrefix, enableILM)
		if err != nil {
			return "", err
		}
	} else {
		mapping, err = fixMappingFunc(esTemplate.TextTemplateBuilder{}, loadMappingFunc("/"+mappingName+".json"), shards, replicas, "", false)
		if err != nil {
			return "", err
		}
	}
	return mapping, nil

}

func isValidOption(val string) bool {
	allowedValues := []string{"jaeger-span", "jaeger-service"}
	for _, value := range allowedValues {
		if val == value {
			return true
		}
	}
	return false
}
