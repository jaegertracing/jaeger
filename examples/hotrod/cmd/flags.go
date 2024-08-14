// Copyright (c) 2023 The Jaeger Authors.
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

package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

var (
	otelExporter string // otlp, stdout
	verbose      bool

	fixDBConnDelay         time.Duration
	fixDBConnDisableMutex  bool
	fixRouteWorkerPoolSize int

	customerPort int
	driverPort   int
	frontendPort int
	routePort    int

	basepath string
	jaegerUI string
)

// used by root command
func addFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&otelExporter, "otel-exporter", "x", "otlp", "OpenTelemetry exporter (otlp|stdout)")

	cmd.PersistentFlags().DurationVarP(&fixDBConnDelay, "fix-db-query-delay", "D", 300*time.Millisecond, "Average latency of MySQL DB query")
	cmd.PersistentFlags().BoolVarP(&fixDBConnDisableMutex, "fix-disable-db-conn-mutex", "M", false, "Disables the mutex guarding db connection")
	cmd.PersistentFlags().IntVarP(&fixRouteWorkerPoolSize, "fix-route-worker-pool-size", "W", 3, "Default worker pool size")

	// Add flags to choose ports for services
	cmd.PersistentFlags().IntVarP(&customerPort, "customer-service-port", "c", 8081, "Port for customer service")
	cmd.PersistentFlags().IntVarP(&driverPort, "driver-service-port", "d", 8082, "Port for driver service")
	cmd.PersistentFlags().IntVarP(&frontendPort, "frontend-service-port", "f", 8080, "Port for frontend service")
	cmd.PersistentFlags().IntVarP(&routePort, "route-service-port", "r", 8083, "Port for routing service")

	// Flag for serving frontend at custom basepath url
	cmd.PersistentFlags().StringVarP(&basepath, "basepath", "b", "", `Basepath for frontend service(default "/")`)
	cmd.PersistentFlags().StringVarP(&jaegerUI, "jaeger-ui", "j", "http://localhost:16686", "Address of Jaeger UI to create [find trace] links")

	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enables debug logging")
}
