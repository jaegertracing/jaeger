// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package features

import (
	"fmt"

	"go.opentelemetry.io/collector/featuregate"
)

func DisplayFeatures() string {
	f := featuregate.GlobalRegistry()
	if f == nil {
		return ""
	}
	out := ""
	f.VisitAll(func(g *featuregate.Gate) {
		out += fmt.Sprintf("Feature:\t%s\n", g.ID())
		if g.IsEnabled() {
			out += fmt.Sprintf("Default state:\t%s\n", "On")
		} else {
			out += fmt.Sprintf("Default state:\t%s\n", "Off")
		}
		out += fmt.Sprintf("Description:\t%s\n", g.Description())
		if ref := g.ReferenceURL(); ref != "" {
			out += fmt.Sprintf("ReferenceURL:\t%s\n", ref)
		}
		out += "\n"
	})
	return out
}
