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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func clearEnv() {
	os.Setenv(SpanStorageTypeEnvVar, "")
	os.Setenv(DependencyStorageTypeEnvVar, "")
}

func TestFactoryConfigFromEnv(t *testing.T) {
	clearEnv()
	defer clearEnv()

	f := FactoryConfigFromEnvAndCLI(nil, nil)
	assert.Equal(t, 1, len(f.SpanWriterTypes))
	assert.Equal(t, cassandraStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, cassandraStorageType, f.SpanReaderType)
	assert.Equal(t, cassandraStorageType, f.DependenciesStorageType)

	os.Setenv(SpanStorageTypeEnvVar, elasticsearchStorageType)
	os.Setenv(DependencyStorageTypeEnvVar, memoryStorageType)

	f = FactoryConfigFromEnvAndCLI(nil, nil)
	assert.Equal(t, 1, len(f.SpanWriterTypes))
	assert.Equal(t, elasticsearchStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, elasticsearchStorageType, f.SpanReaderType)
	assert.Equal(t, memoryStorageType, f.DependenciesStorageType)

	os.Setenv(SpanStorageTypeEnvVar, elasticsearchStorageType+","+kafkaStorageType)

	f = FactoryConfigFromEnvAndCLI(nil, &bytes.Buffer{})
	assert.Equal(t, 2, len(f.SpanWriterTypes))
	assert.Equal(t, []string{elasticsearchStorageType, kafkaStorageType}, f.SpanWriterTypes)
	assert.Equal(t, elasticsearchStorageType, f.SpanReaderType)
}

func TestFactoryConfigFromEnvDeprecated(t *testing.T) {
	clearEnv()

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
		assert.Equal(t, 1, len(f.SpanWriterTypes))
		assert.Equal(t, testCase.value, f.SpanWriterTypes[0])
		assert.Equal(t, testCase.value, f.SpanReaderType)
		assert.Equal(t, testCase.value, f.DependenciesStorageType)
		if testCase.log {
			expectedLog := "WARNING: found deprecated command line option"
			assert.Equal(t, expectedLog, log.String()[0:len(expectedLog)])
		}
	}
}
