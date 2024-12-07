// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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

	// SamplingStorageTypeEnvVar is the name of the env var that defines the type of backend used for sampling data storage when using adaptive sampling.
	SamplingStorageTypeEnvVar = "SAMPLING_STORAGE_TYPE"

	spanStorageFlag = "--span-storage.type"
)

// FactoryConfig tells the Factory which types of backends it needs to create for different storage types.
type FactoryConfig struct {
	SpanWriterTypes         []string
	SpanReaderType          string
	SamplingStorageType     string
	DependenciesStorageType string
	DownsamplingRatio       float64
	DownsamplingHashSalt    string
}

// FactoryConfigFromEnvAndCLI reads the desired types of storage backends from SPAN_STORAGE_TYPE and
// DEPENDENCY_STORAGE_TYPE environment variables. Allowed values:
// * `cassandra` - built-in
// * `opensearch` - built-in
// * `elasticsearch` - built-in
// * `memory` - built-in
// * `kafka` - built-in
// * `blackhole` - built-in
// * `grpc` - build-in
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
	samplingStorageType := os.Getenv(SamplingStorageTypeEnvVar)
	// TODO support explicit configuration for readers
	return FactoryConfig{
		SpanWriterTypes:         spanWriterTypes,
		SpanReaderType:          spanWriterTypes[0],
		DependenciesStorageType: depStorageType,
		SamplingStorageType:     samplingStorageType,
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
