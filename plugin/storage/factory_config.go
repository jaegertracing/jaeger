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
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// SpanStorageTypeEnvVar is the name of the env var that defines the type of backend used for span storage.
	SpanStorageTypeEnvVar = "SPAN_STORAGE_TYPE"

	// DependencyStorageTypeEnvVar is the name of the env var that defines the type of backend used for dependencies storage.
	DependencyStorageTypeEnvVar = "DEPENDENCY_STORAGE_TYPE"

	spanStorageFlag = "--span-storage.type"
)

// FactoryConfig tells the Factory which types of backends it needs to create for different storage types.
type FactoryConfig struct {
	SpanWriterTypes         []string
	SpanReaderType          string
	DependenciesStorageType string
}

// FactoryConfigFromEnvAndCLI reads the desired types of storage backends from SPAN_STORAGE_TYPE and
// DEPENDENCY_STORAGE_TYPE environment variables. Allowed values:
//   * `cassandra` - built-in
//   * `elasticsearch` - built-in
//   * `memory` - built-in
//   * `kafka` - built-in
//   * `plugin` - loads a dynamic plugin that implements storage.Factory interface (not supported at the moment)
//
// For backwards compatibility it also parses the args looking for deprecated --span-storage.type flag.
// If found, it writes a deprecation warning to the log.
func FactoryConfigFromEnvAndCLI(args []string, log io.Writer) FactoryConfig {
	spanStorageType := os.Getenv(SpanStorageTypeEnvVar)
	if spanStorageType == "" {
		// for backwards compatibility check command line for --span-storage.type flag
		spanStorageType = spanStorageTypeFromArgs(args, log)
	}
	if spanStorageType == "" {
		spanStorageType = cassandraStorageType
	}
	spanWriterTypes := strings.Split(spanStorageType, ",")
	if len(spanWriterTypes) > 1 {
		fmt.Fprintf(log,
			"WARNING: multiple span storage types have been specified. "+
				"Only the first type (%s) will be used for reading and archiving.\n\n",
			spanWriterTypes[0],
		)
	}
	depStorageType := os.Getenv(DependencyStorageTypeEnvVar)
	if depStorageType == "" {
		depStorageType = spanWriterTypes[0]
	}
	// TODO support explicit configuration for readers
	return FactoryConfig{
		SpanWriterTypes:         spanWriterTypes,
		SpanReaderType:          spanWriterTypes[0],
		DependenciesStorageType: depStorageType,
	}
}

func spanStorageTypeFromArgs(args []string, log io.Writer) string {
	for i, token := range args {
		if i == 0 {
			continue // skip app name; easier than dealing with +-1 offset
		}
		if !strings.HasPrefix(token, spanStorageFlag) {
			continue
		}
		fmt.Fprintf(
			log,
			"WARNING: found deprecated command line option %s, please use environment variable %s instead\n",
			token,
			SpanStorageTypeEnvVar,
		)
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
