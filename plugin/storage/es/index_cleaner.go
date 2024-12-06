// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

type CleanerConfigWrapper struct {
	*config.Configuration
}

func (cfg *CleanerConfigWrapper) RunCleaner() {
	if !cfg.IndexCleaner.Enabled {
		log.Println("Cleaner is disabled, skipping...")
		return
	}

	ticker := time.NewTicker(cfg.IndexCleaner.Frequency)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Running index cleaner...")

		// Call for existing cmd/es-index-cleaner logic here
		err := cfg.runESIndexCleaner()
		if err != nil {
			log.Printf("Error running es-index-cleaner: %v", err)
		}
	}
}

// runESIndexCleaner runs the cmd/es-index-cleaner command.
func (cfg *CleanerConfigWrapper) runESIndexCleaner() error {
	if len(cfg.Servers) == 0 {
		return errors.New("elasticsearch server URL is missing in configuration")
	}

	cmd := exec.Command("./cmd/es-index-cleaner")

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to run es-index-cleaner: %w", err)
	}

	log.Println("Index cleaner executed successfully.")
	return nil
}
