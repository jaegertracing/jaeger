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
	"os"
	"strings"

	"go.uber.org/zap"
)

const (
	// SpanStorageTypeEnvVar is the name of the env var that defines the type of backend used for span writing.
	SpanStorageTypeEnvVar = "SPAN_STORAGE_TYPE"

	// SpanReaderTypeEnvVar is the optional env var that defines the type of backend used for span reading in the case of multi-storage.
	SpanReaderTypeEnvVar = "SPAN_READER_TYPE"

	// DependencyStorageTypeEnvVar is the name of the env var that defines the type of backend used for dependencies storage.
	DependencyStorageTypeEnvVar = "DEPENDENCY_STORAGE_TYPE"

	spanStorageFlag = "--span-storage.type"
)

// FactoryConfig tells the Factory which types of backends it needs to create for different storage types.
type FactoryConfig struct {
	SpanWriterTypes         []string
	SpanReaderType          string
	DependenciesStorageType string
	logger                  *zap.Logger
}

// FactoryConfigFromEnvAndCLI reads the desired types of storage backends from SPAN_STORAGE_TYPE,
// DEPENDENCY_STORAGE_TYPE and DEPENDENCY_STORAGE_TYPE environment variables. Allowed values:
//   * `cassandra` - built-in
//   * `elasticsearch` - built-in
//   * `memory` - built-in
//   * `kafka` - built-in
//   * `plugin` - loads a dynamic plugin that implements storage.Factory interface (not supported at the moment)
//
// For backwards compatibility it also parses the args looking for deprecated --span-storage.type flag.
// If found, it writes a deprecation warning to the log.
func FactoryConfigFromEnvAndCLI(args []string, logger *zap.Logger) FactoryConfig {
	spanStorageType := os.Getenv(SpanStorageTypeEnvVar)
	if spanStorageType == "" {
		// for backwards compatibility check command line for --span-storage.type flag
		spanStorageType = spanStorageTypeFromArgs(args, logger)
	}
	if spanStorageType == "" {
		spanStorageType = cassandraStorageType
	}
	spanWriterTypes := strings.Split(spanStorageType, ",")

	spanReaderType := os.Getenv(SpanReaderTypeEnvVar)
	if spanReaderType == "" {
		if len(spanWriterTypes) > 1 {
			logger.Warn("the first span storage type listed (" + spanWriterTypes[0] + ") will be used for reading. " +
				"Please use environment variable " + SpanReaderTypeEnvVar + " to specify which storage type to read from")
		}
		spanReaderType = spanWriterTypes[0]
	}

	depStorageType := os.Getenv(DependencyStorageTypeEnvVar)
	if depStorageType == "" {
		depStorageType = spanReaderType
	}
	return FactoryConfig{
		SpanWriterTypes:         spanWriterTypes,
		SpanReaderType:          spanReaderType,
		DependenciesStorageType: depStorageType,
		logger:                  logger,
	}
}

func spanStorageTypeFromArgs(args []string, logger *zap.Logger) string {
	for i, token := range args {
		if i == 0 {
			continue // skip app name; easier than dealing with +-1 offset
		}
		if !strings.HasPrefix(token, spanStorageFlag) {
			continue
		}
		logger.Warn("found deprecated command line option " + token +
			", please use environment variable " + SpanStorageTypeEnvVar + " instead")
		if token == spanStorageFlag && i < len(args)-1 {
			return args[i+1]
		}
		if strings.HasPrefix(token, spanStorageFlag+"=") {
			return token[(len(spanStorageFlag) + 1):]
		}
		break
	}
	return ""
}
