// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/examples/hotrod/services/config"
	"github.com/jaegertracing/jaeger/internal/jaegerclientenv2otel"
	"github.com/jaegertracing/jaeger/internal/metrics/prometheus"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

var (
	logger         *zap.Logger
	metricsFactory metrics.Factory
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "examples-hotrod",
	Short: "HotR.O.D. - A tracing demo application",
	Long:  `HotR.O.D. - A tracing demo application.`,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		logger.Fatal("We bowled a googly", zap.Error(err))
		os.Exit(-1)
	}
}

func init() {
	addFlags(RootCmd)
	cobra.OnInitialize(onInitialize)
}

// onInitialize is called before the command is executed.
func onInitialize() {
	zapOptions := []zap.Option{
		zap.AddStacktrace(zapcore.FatalLevel),
		zap.AddCallerSkip(1),
	}
	if !verbose {
		zapOptions = append(zapOptions,
			zap.IncreaseLevel(zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l != zapcore.DebugLevel })),
		)
	}
	logger, _ = zap.NewDevelopment(zapOptions...)

	jaegerclientenv2otel.MapJaegerToOtelEnvVars(logger)

	metricsFactory = prometheus.New().Namespace(metrics.NSOptions{Name: "hotrod", Tags: nil})

	if config.MySQLGetDelay != fixDBConnDelay {
		logger.Info("fix: overriding MySQL query delay", zap.Duration("old", config.MySQLGetDelay), zap.Duration("new", fixDBConnDelay))
		config.MySQLGetDelay = fixDBConnDelay
	}
	if fixDBConnDisableMutex {
		logger.Info("fix: disabling db connection mutex")
		config.MySQLMutexDisabled = true
	}
	if config.RouteWorkerPoolSize != fixRouteWorkerPoolSize {
		logger.Info("fix: overriding route worker pool size", zap.Int("old", config.RouteWorkerPoolSize), zap.Int("new", fixRouteWorkerPoolSize))
		config.RouteWorkerPoolSize = fixRouteWorkerPoolSize
	}

	if customerPort != 8081 {
		logger.Info("changing customer service port", zap.Int("old", 8081), zap.Int("new", customerPort))
	}

	if driverPort != 8082 {
		logger.Info("changing driver service port", zap.Int("old", 8082), zap.Int("new", driverPort))
	}

	if frontendPort != 8080 {
		logger.Info("changing frontend service port", zap.Int("old", 8080), zap.Int("new", frontendPort))
	}

	if routePort != 8083 {
		logger.Info("changing route service port", zap.Int("old", 8083), zap.Int("new", routePort))
	}

	if basepath != "" {
		logger.Info("changing basepath for frontend", zap.String("old", "/"), zap.String("new", basepath))
	}
}

func logError(logger *zap.Logger, err error) error {
	if err != nil {
		logger.Error("Error running command", zap.Error(err))
	}
	return err
}
