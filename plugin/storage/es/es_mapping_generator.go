// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// NewEsMappingsCommand defines the new CLI command to run esmapping-generator
func NewEsMappingsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "es-mappings",
		Short: "Print Elasticsearch index mappings to stdout",
		Run: func(_ *cobra.Command, _ []string) {
			if _, err := os.Stat("./cmd/esmapping-generator"); os.IsNotExist(err) {
				log.Fatalf("esmapping-generator not found: %v", err)
			}

			// Run the esmapping-generator command
			execCmd := exec.Command("go", "run", "./cmd/esmapping-generator")
			output, err := execCmd.CombinedOutput()
			if err != nil {
				log.Printf("Error executing esmapping-generator: %v\nOutput: %s", err, output)
				return
			}
			fmt.Println("esmapping-generator executed successfully")
			fmt.Printf("%s\n", output)
		},
	}
}
