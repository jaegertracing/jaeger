// Copyright (c) 2017 Uber Technologies, Inc.
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
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	jprom "github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"
)

var (
	metricsBackend string
	jAgentHostPort string
	logger         *zap.Logger
	metricsFactory metrics.Factory
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "jaeger-demo",
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
	RootCmd.PersistentFlags().StringVarP(&metricsBackend, "metrics", "m", "expvar", "Metrics backend (expvar|prometheus)")
	RootCmd.PersistentFlags().StringVarP(&jAgentHostPort, "jaeger-agent.host-port", "a", "0.0.0.0:6831", "String representing jaeger-agent host:port")
	rand.Seed(int64(time.Now().Nanosecond()))
	logger, _ = zap.NewDevelopment()
	cobra.OnInitialize(initMetrics)
}

// initMetrics is called before the command is executed.
func initMetrics() {
	if metricsBackend == "expvar" {
		metricsFactory = xkit.Wrap("", expvar.NewFactory(10)) // 10 buckets for histograms
		logger.Info("Using expvar as metrics backend")
	} else if metricsBackend == "prometheus" {
		metricsFactory = jprom.New()
		logger.Info("Using Prometheus as metrics backend")
	} else {
		logger.Fatal("unsupported metrics backend " + metricsBackend)
	}
}
