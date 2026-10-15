// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"fmt"
	"path/filepath"
	"reflect"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
)

// Generate produces a JSON Schema describing the default configuration of every
// Jaeger v2 component factory in factories.  The returned schema is deterministic:
// object properties and $defs are emitted in sorted key order and required arrays
// are sorted.
func Generate(factories otelcol.Factories) (*Schema, error) {
	g := newGenerator()
	if err := g.docs.load(modulePath(), sourceDirs()); err != nil {
		return nil, fmt.Errorf("loading documentation: %w", err)
	}
	return g.generate(factories)
}

func (g *generator) generate(factories otelcol.Factories) (*Schema, error) {
	root := &Schema{
		Schema:     "https://json-schema.org/draft/2020-12/schema",
		Type:       "object",
		Title:      "Jaeger v2 component configuration schema",
		Properties: map[string]*Schema{},
		Defs:       g.defs,
	}

	entries := []struct {
		kind string
		m    map[component.Type]component.Factory
	}{
		{"extension", toGenericMap(factories.Extensions)},
		{"receiver", toGenericMap(factories.Receivers)},
		{"processor", toGenericMap(factories.Processors)},
		{"exporter", toGenericMap(factories.Exporters)},
		{"connector", toGenericMap(factories.Connectors)},
	}

	for _, e := range entries {
		for typ, factory := range e.m {
			cfg := factory.CreateDefaultConfig()
			v := reflect.ValueOf(cfg)
			if v.Kind() == reflect.Pointer {
				if v.IsNil() {
					v = reflect.New(v.Type().Elem())
				}
				v = v.Elem()
			}
			if v.Kind() != reflect.Struct {
				continue
			}

			root.Properties[fmt.Sprintf("%s/%s", e.kind, typ.String())] = g.typeToSchema(v.Type(), v)
		}
	}

	return root, nil
}

func toGenericMap[T component.Factory](m map[component.Type]T) map[component.Type]component.Factory {
	out := make(map[component.Type]component.Factory, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// sourceDirs returns the Jaeger source directories that should be scanned for
// doc comments.
func sourceDirs() []string {
	root := moduleRoot()
	return []string{
		filepath.Join(root, "cmd", "jaeger", "internal"),
		filepath.Join(root, "cmd", "internal"),
		filepath.Join(root, "components"),
		filepath.Join(root, "internal"),
	}
}
