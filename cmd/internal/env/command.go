// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package env

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/jaegertracing/jaeger/internal/storage/metricstore"
	storage "github.com/jaegertracing/jaeger/internal/storage/v1/factory"
)

const (
	longTemplate = `
All command line options can be provided via environment variables by converting
their names to upper case and replacing punctuation with underscores. For example:

command line option                 environment variable
------------------------------------------------------------------
--cassandra.connections-per-host    CASSANDRA_CONNECTIONS_PER_HOST
--metrics-backend                   METRICS_BACKEND

The following configuration options are only available via environment variables:
%s
`
	storageTypeDescription = `The type of backend [%s] used for trace storage.
Multiple backends can be specified as comma-separated list, e.g. "cassandra,elasticsearch"
(currently only for writing spans). Note that "kafka" is only valid in jaeger-collector;
it is not a replacement for a proper storage backend, and only used as a buffer for spans
when Jaeger is deployed in the collector+ingester configuration.
`

	metricsStorageTypeDescription = `The type of backend [%s] used as a metrics store with
Service Performance Monitoring (https://www.jaegertracing.io/docs/latest/spm/).
`
)

// Command creates `env` command
func Command() *cobra.Command {
	fs := new(pflag.FlagSet)
	fs.String(
		storage.SpanStorageTypeEnvVar,
		"cassandra",
		fmt.Sprintf(
			strings.ReplaceAll(storageTypeDescription, "\n", " "),
			strings.Join(storage.AllStorageTypes, ", "),
		),
	)
	fs.String(
		storage.DependencyStorageTypeEnvVar,
		"${SPAN_STORAGE_TYPE}",
		"The type of backend used for service dependencies storage.",
	)
	fs.String(
		metricstore.StorageTypeEnvVar,
		"",
		fmt.Sprintf(
			strings.ReplaceAll(metricsStorageTypeDescription, "\n", " "),
			strings.Join(metricstore.AllStorageTypes, ", "),
		),
	)
	long := fmt.Sprintf(longTemplate, strings.ReplaceAll(fs.FlagUsagesWrapped(0), "      --", "\n"))
	return &cobra.Command{
		Use:   "env",
		Short: "Help about environment variables.",
		Long:  long,
		Run: func(cmd *cobra.Command, _ /* args */ []string) {
			fmt.Fprint(cmd.OutOrStdout(), long)
		},
	}
}
