// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/spf13/cobra"
)

func NewEsMappingsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "es-mappings",
		Short: "Print Elasticsearch index mappings to stdout",
		Run: func(_ *cobra.Command, _ []string) {
			execCmd := exec.Command("go", "run", "./cmd/esmapping-generator")
			err := execCmd.Run()
			if err != nil {
				log.Fatalf("Error executing esmapping-generator: %v", err)
			}
			fmt.Println("esmapping-generator executed successfully")
		},
	}
}
