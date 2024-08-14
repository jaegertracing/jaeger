// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
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

package storage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFactoryConfigFromEnv(t *testing.T) {
	f := FactoryConfigFromEnvAndCLI(nil, &bytes.Buffer{})
	assert.Len(t, f.SpanWriterTypes, 1)
	assert.Equal(t, cassandraStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, cassandraStorageType, f.SpanReaderType)
	assert.Equal(t, cassandraStorageType, f.DependenciesStorageType)
	assert.Empty(t, f.SamplingStorageType)

	t.Setenv(SpanStorageTypeEnvVar, elasticsearchStorageType)
	t.Setenv(DependencyStorageTypeEnvVar, memoryStorageType)
	t.Setenv(SamplingStorageTypeEnvVar, cassandraStorageType)

	f = FactoryConfigFromEnvAndCLI(nil, &bytes.Buffer{})
	assert.Len(t, f.SpanWriterTypes, 1)
	assert.Equal(t, elasticsearchStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, elasticsearchStorageType, f.SpanReaderType)
	assert.Equal(t, memoryStorageType, f.DependenciesStorageType)
	assert.Equal(t, cassandraStorageType, f.SamplingStorageType)

	t.Setenv(SpanStorageTypeEnvVar, elasticsearchStorageType+","+kafkaStorageType)

	f = FactoryConfigFromEnvAndCLI(nil, &bytes.Buffer{})
	assert.Len(t, f.SpanWriterTypes, 2)
	assert.Equal(t, []string{elasticsearchStorageType, kafkaStorageType}, f.SpanWriterTypes)
	assert.Equal(t, elasticsearchStorageType, f.SpanReaderType)

	t.Setenv(SpanStorageTypeEnvVar, badgerStorageType)

	f = FactoryConfigFromEnvAndCLI(nil, nil)
	assert.Len(t, f.SpanWriterTypes, 1)
	assert.Equal(t, badgerStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, badgerStorageType, f.SpanReaderType)
}

func TestFactoryConfigFromEnvDeprecated(t *testing.T) {
	testCases := []struct {
		args  []string
		log   bool
		value string
	}{
		{args: []string{"appname", "-x", "y", "--span-storage.type=memory"}, log: true, value: "memory"},
		{args: []string{"appname", "-x", "y", "--span-storage.type", "memory"}, log: true, value: "memory"},
		{args: []string{"appname", "-x", "y", "--span-storage.type"}, log: true, value: "cassandra"},
		{args: []string{"appname", "-x", "y"}, log: false, value: "cassandra"},
	}
	for _, testCase := range testCases {
		log := new(bytes.Buffer)
		f := FactoryConfigFromEnvAndCLI(testCase.args, log)
		assert.Len(t, f.SpanWriterTypes, 1)
		assert.Equal(t, testCase.value, f.SpanWriterTypes[0])
		assert.Equal(t, testCase.value, f.SpanReaderType)
		assert.Equal(t, testCase.value, f.DependenciesStorageType)
		if testCase.log {
			expectedLog := "WARNING: found deprecated command line option"
			assert.Equal(t, expectedLog, log.String()[0:len(expectedLog)])
		}
	}
}
