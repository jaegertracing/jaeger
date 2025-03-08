// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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

	customerHostname string
	driverHostname   string
	frontendHostname string
	routeHostname    string

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

	// Add flags to choose hostnames for services
	cmd.PersistentFlags().StringVar(&customerHostname, "customer-service-hostname", "0.0.0.0", "Hostname for customer service")
	cmd.PersistentFlags().StringVar(&driverHostname, "driver-service-hostname", "0.0.0.0", "Hostname for driver service")
	cmd.PersistentFlags().StringVar(&frontendHostname, "frontend-service-hostname", "0.0.0.0", "Hostname for frontend service")
	cmd.PersistentFlags().StringVar(&routeHostname, "route-service-hostname", "0.0.0.0", "Hostname for routing service")

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
