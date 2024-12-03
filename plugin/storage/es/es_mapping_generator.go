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
		Run:   runESMappings,
	}
}

func runESMappings(cmd *cobra.Command, args []string) {
	execCmd := exec.Command("go", "run", "./cmd/esmapping-generator")
	err := execCmd.Run()
	if err != nil {
		log.Fatalf("Error executing esmapping-generator: %v", err)
	}

	fmt.Println("esmapping-generator executed successfully")
}
