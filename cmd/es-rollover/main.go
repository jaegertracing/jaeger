// Copyright (c) 2021 The Jaeger Authors.
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

package main

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	initialize "github.com/jaegertracing/jaeger/cmd/es-rollover/app/init"
	"github.com/jaegertracing/jaeger/cmd/es-rollover/app/lookback"
	"github.com/jaegertracing/jaeger/cmd/es-rollover/app/rollover"
)

func main() {
	v := viper.New()
	logger, _ := zap.NewProduction()

	var rootCmd = &cobra.Command{
		Use:   "jaeger-es-rollover ACTION http://HOSTNAME:PORT",
		Short: "Jaeger es-rollover manages Jaeger indices",
		Long:  "Jaeger es-rollover manages Jaeger indices",
	}

	// Configure init command
	rootCmd.AddCommand(initialize.Command(v, logger))
	rootCmd.AddCommand(rollover.Command(v, logger))
	rootCmd.AddCommand(lookback.Command(v, logger))

	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
