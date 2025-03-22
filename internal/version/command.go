// Copyright (c) 2017 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

// Command creates version command
func Command() *cobra.Command {
	info := Get()
	log.Println("application version:", info)
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version.",
		Long:  `Print the version and build information.`,
		RunE: func(cmd *cobra.Command, _ /* args */ []string) error {
			json, err := json.Marshal(info)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), string(json))
			return nil
		},
	}
}
