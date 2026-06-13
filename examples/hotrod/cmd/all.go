// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/cobra"
)

// allCmd represents the all command
var allCmd = &cobra.Command{
	Use:   "all",
	Short: "Starts all services",
	Long:  `Starts all services.`,
	RunE: func(_ *cobra.Command, args []string) error {
		logger.Info("Starting all services")
		go customerCmd.RunE(customerCmd, args)
		go driverCmd.RunE(driverCmd, args)
		go routeCmd.RunE(routeCmd, args)
		return frontendCmd.RunE(frontendCmd, args)
	},
}

func init() {
	RootCmd.AddCommand(allCmd)
}
