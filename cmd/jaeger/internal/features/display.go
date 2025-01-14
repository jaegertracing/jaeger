// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package features

import (
	"fmt"

	"go.opentelemetry.io/collector/featuregate"
)

func DisplayFeatures() {
	f := featuregate.GlobalRegistry()
	if f == nil {
		return
	}
	f.VisitAll(func(g *featuregate.Gate) {
		fmt.Printf("Feature:\t%s\n", g.ID())
		if !g.IsEnabled() {
			fmt.Printf("Default state:\t%s\n", "On")
		} else {
			fmt.Printf("Default state:\t%s\n", "Off")
		}
		fmt.Printf("Description:\t%s\n", g.Description())

		if ref := g.ReferenceURL(); ref != "" {
			fmt.Printf("ReferenceURL:\t%s\n", ref)
		}
		fmt.Println()
	})
}
