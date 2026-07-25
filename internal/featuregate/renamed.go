// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package featuregate contains helpers for Jaeger feature gates.
package featuregate

import (
	"fmt"
	"io"
	"os"
	"sync"

	collectorfeaturegate "go.opentelemetry.io/collector/featuregate"
)

// RenamedGate keeps a legacy feature gate ID working while users migrate to a
// replacement ID. Alpha gates combine the IDs with OR because they default to
// disabled. Beta gates combine them with AND because they default to enabled.
type RenamedGate struct {
	current  *collectorfeaturegate.Gate
	legacy   *collectorfeaturegate.Gate
	warnings io.Writer
	warnOnce sync.Once
}

// NewRenamedGate creates a feature gate backed by current and legacy IDs.
func NewRenamedGate(current, legacy *collectorfeaturegate.Gate) *RenamedGate {
	if current.Stage() != legacy.Stage() {
		panic("renamed feature gate IDs must use the same stage")
	}
	if current.Stage() != collectorfeaturegate.StageAlpha && current.Stage() != collectorfeaturegate.StageBeta {
		panic("only Alpha and Beta feature gates can be renamed")
	}
	return &RenamedGate{
		current: current,
		legacy:  legacy,
		// Feature gates can be evaluated during configuration validation,
		// before an application logger is available.
		warnings: os.Stderr,
	}
}

// ID returns the replacement feature gate ID.
func (g *RenamedGate) ID() string {
	return g.current.ID()
}

// LegacyID returns the deprecated feature gate ID.
func (g *RenamedGate) LegacyID() string {
	return g.legacy.ID()
}

// IsEnabled returns the effective state across the replacement and legacy IDs.
func (g *RenamedGate) IsEnabled() bool {
	currentEnabled := g.current.IsEnabled()
	legacyEnabled := g.legacy.IsEnabled()

	var enabled, legacySelected bool
	if g.current.Stage() == collectorfeaturegate.StageAlpha {
		enabled = currentEnabled || legacyEnabled
		legacySelected = legacyEnabled && !currentEnabled
	} else {
		enabled = currentEnabled && legacyEnabled
		legacySelected = !legacyEnabled && currentEnabled
	}

	if legacySelected {
		g.warnOnce.Do(func() {
			fmt.Fprintf(g.warnings, "Feature gate %q has been renamed to %q; the legacy ID will be removed in a future release.\n", g.LegacyID(), g.ID())
		})
	}
	return enabled
}
