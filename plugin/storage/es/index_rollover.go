// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"log"
	"os/exec"
	"time"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

type RolloverConfigWrapper struct {
	*config.Configuration
}

// RunRollover schedules and runs the rollover process based on the configured frequency.
func (cfg *RolloverConfigWrapper) RunRollover() {
	if !cfg.Rollover.Enabled {
		log.Println("Rollover is disabled, skipping...")
		return
	}

	ticker := time.NewTicker(cfg.Rollover.Frequency)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Running index rollover...")

		cmd := exec.Command("./cmd/es-rollover")
		err := cmd.Run()

		if err != nil {
			log.Printf("Error running es-rollover: %v", err)
		} else {
			log.Println("Index rollover executed successfully.")
		}
	}
}
