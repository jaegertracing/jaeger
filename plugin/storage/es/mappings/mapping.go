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
	"bytes"

	"github.com/jaegertracing/jaeger/pkg/es"
)

// MappingBuilder holds parameters required to render an elasticsearch index template
type MappingBuilder struct {
	templateBuilder es.TemplateBuilder
	shards          int64
	replicas        int64
	esVersion       uint
	esPrefix        string
	useILM          bool
}

// NewBuilder constructs and returns an initialized MappingBuilder
func NewBuilder(tb es.TemplateBuilder, shards, replicas int64, esVersion uint, esPrefix string, useILM bool) *MappingBuilder {
	return &MappingBuilder{templateBuilder: tb,
		shards: shards, replicas: replicas,
		esVersion: esVersion, esPrefix: esPrefix,
		useILM: useILM,
	}
}

// GetMapping returns the render mapping based on elasticsearch version
func (mb *MappingBuilder) GetMapping(mapping string) (string, error) {
	if mb.esVersion == 7 {
		return mb.fixMapping("/" + mapping + "-7.json")
	}
	return mb.fixMapping("/" + mapping + ".json")
}

// GetSpanServiceMappings returns span and service mappings
func (mb *MappingBuilder) GetSpanServiceMappings() (string, string, error) {
	spanMapping, err := mb.GetMapping("span")
	if err != nil {
		return "", "", err
	}
	serviceMapping, err := mb.GetMapping("service")
	if err != nil {
		return "", "", err
	}
	return spanMapping, serviceMapping, nil
}

// GetDependenciesMappings returns dependencies mappings
func (mb *MappingBuilder) GetDependenciesMappings() (string, error) {
	return mb.GetMapping("dependencies")
}

func loadMapping(name string) string {
	s, _ := FSString(false, name)
	return s
}

func (mb *MappingBuilder) fixMapping(mapping string) (string, error) {

	tmpl, err := mb.templateBuilder.Parse(loadMapping(mapping))
	if err != nil {
		return "", err
	}
	writer := new(bytes.Buffer)

	if mb.esPrefix != "" {
		mb.esPrefix += "-"
	}
	values := struct {
		NumberOfShards   int64
		NumberOfReplicas int64
		ESPrefix         string
		UseILM           bool
	}{mb.shards, mb.replicas, mb.esPrefix, mb.useILM}

	if err := tmpl.Execute(writer, values); err != nil {
		return "", err
	}

	return writer.String(), nil
}
