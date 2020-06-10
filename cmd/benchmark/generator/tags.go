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

package generator

import (
	"math"
	"math/rand"

	"github.com/jaegertracing/jaeger/cmd/benchmark/generator/data"
	"github.com/jaegertracing/jaeger/model"
)

const tagSeparator = "."

var tagTypes = []model.ValueType{
	model.ValueType_INT64,
	model.ValueType_STRING,
	model.ValueType_BOOL,
	model.ValueType_FLOAT64,
}

type Process struct {
	Id      string
	Process *model.Process
}

type TagTemplate struct {
	Key   string
	Type  model.ValueType
	words []string
}

func (t *TagTemplate) Tag() model.KeyValue {
	tag := model.KeyValue{
		Key:   t.Key,
		VType: t.Type,
	}
	switch t.Type {
	case model.ValueType_INT64:
		tag.VInt64 = rand.Int63n(math.MaxInt64)
	case model.ValueType_FLOAT64:
		tag.VFloat64 = rand.Float64() * math.MaxFloat64
	case model.ValueType_BOOL:
		tag.VBool = rand.Intn(2) == 0
	case model.ValueType_STRING:
		tag.VStr = t.words[rand.Intn(len(t.words)-1)]
	}
	return tag
}

func generateTagTemplates(max int, words []string) []*TagTemplate {
	tags := make([]*TagTemplate, max)
	keys := generateRandStrings(data.Tags, tagSeparator, max)
	ntypes := len(tagTypes) - 1
	for i := 0; i < len(tags); i++ {
		t := rand.Intn(ntypes)
		tags[i] = &TagTemplate{
			Key:   keys[i],
			Type:  tagTypes[t],
			words: words,
		}
	}

	return tags
}

func generateTagsFromPool(pool []*TagTemplate, min int) []model.KeyValue {
	max := len(pool)
	size := rand.Intn(max-min) + min
	tags := make([]model.KeyValue, size)
	for i := 0; i < size; i++ {
		index := rand.Intn(max - 1)
		tag := pool[index]
		tags[i] = tag.Tag()
	}
	return tags
}
