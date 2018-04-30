// Copyright (c) 2018 The Jaeger Authors.
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

package env

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/jaegertracing/jaeger/plugin/storage"
)

// Command creates `env` command
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Help about environment variables",
		Long:  `Help about environment variables`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), `
All command line options can be provided via environment variables by converting
their names to upper case and replacing punctuation with underscores. For example,

      command line option                 environment variable
      ------------------------------------------------------------------
      --cassandra.connections-per-host    CASSANDRA_CONNECTIONS_PER_HOST
      --metrics-backend                   METRICS_BACKEND

The following configuration options are only available via environment variables:`+"\n")
			fs := new(pflag.FlagSet)
			fs.String(storage.SpanStorageTypeEnvVar, "cassandra", "The type of backend (cassandra, elasticsearch, memory) used for trace storage.")
			fs.String(storage.DependencyStorageTypeEnvVar, "${SPAN_STORAGE}", "The type of backend used for service dependencies storage.")
			fmt.Fprintln(cmd.OutOrStdout(), strings.Replace(fs.FlagUsages(), "      --", "      ", -1))
		},
	}
}
