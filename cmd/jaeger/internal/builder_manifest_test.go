// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.yaml.in/yaml/v3"
)

const (
	componentsFile = "components.go"
	manifestFile   = "../builder.yaml"
	jaegerModule   = "github.com/jaegertracing/jaeger/"
)

// TestBuilderManifestMatchesComponents keeps cmd/jaeger/builder.yaml, the reference ocb
// manifest, in step with the component set the standard binary registers. The two once
// disagreed about the e2e-only storage_cleaner, so a distribution assembled from the
// manifest was not the distribution users ran.
//
// The manifest identifies a component only by the package to import, which ocb turns
// into a call to that package's NewFactory. So the comparison is over import paths.
// Go rejects unused imports and registering factories is all components.go does, which
// makes its Jaeger imports the set of components it registers.
func TestBuilderManifestMatchesComponents(t *testing.T) {
	assert.ElementsMatch(t, readManifest(t).imports(), componentImports(t),
		"add the component to both %s and %s, or remove it from both", manifestFile, componentsFile)
}

// TestBuilderManifestSectionSizes checks each manifest section against the factories
// Components() returns for that kind. Import paths alone cannot do this: a component
// moved from one section to another leaves the set of imports unchanged, and a factory
// carries no record of the package that built it.
func TestBuilderManifestSectionSizes(t *testing.T) {
	manifest := readManifest(t)
	factories, err := Components()
	require.NoError(t, err)

	for _, section := range []struct {
		kind       string
		declared   int
		registered int
	}{
		{"extensions", len(manifest.Extensions), countFactories(factories.Extensions)},
		{"receivers", len(manifest.Receivers), countFactories(factories.Receivers)},
		{"exporters", len(manifest.Exporters), countFactories(factories.Exporters)},
		{"processors", len(manifest.Processors), countFactories(factories.Processors)},
		{"connectors", len(manifest.Connectors), countFactories(factories.Connectors)},
	} {
		assert.Equal(t, section.registered, section.declared,
			"%s: %s declares a different number than Components() registers", section.kind, manifestFile)
	}
}

// countFactories counts the distinct factories in a kind's map. MakeFactoryMap gives a
// factory with a deprecated alias two keys, and both point at the one component the
// manifest lists once.
func countFactories[F component.Factory](factories map[component.Type]F) int {
	canonical := make(map[component.Type]struct{}, len(factories))
	for _, f := range factories {
		canonical[f.Type()] = struct{}{}
	}
	return len(canonical)
}

type module struct {
	Import string `yaml:"import"`
}

type manifest struct {
	Telemetry  module   `yaml:"telemetry"`
	Extensions []module `yaml:"extensions"`
	Receivers  []module `yaml:"receivers"`
	Exporters  []module `yaml:"exporters"`
	Processors []module `yaml:"processors"`
	Connectors []module `yaml:"connectors"`
}

// imports returns the import path of every component the manifest declares, across the
// telemetry entry and all five component sections.
func (m manifest) imports() []string {
	imports := []string{m.Telemetry.Import}
	for _, section := range [][]module{m.Extensions, m.Receivers, m.Exporters, m.Processors, m.Connectors} {
		for _, mod := range section {
			imports = append(imports, mod.Import)
		}
	}
	return imports
}

func readManifest(t *testing.T) manifest {
	data, err := os.ReadFile(manifestFile)
	require.NoError(t, err)
	var m manifest
	require.NoError(t, yaml.Unmarshal(data, &m))
	require.NotEmpty(t, m.Extensions, "no components parsed from %s", manifestFile)
	return m
}

// componentImports returns the Jaeger packages that components.go imports, which are
// the components it registers.
func componentImports(t *testing.T) []string {
	file, err := parser.ParseFile(token.NewFileSet(), componentsFile, nil, parser.ImportsOnly)
	require.NoError(t, err)

	var imports []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		require.NoError(t, err)
		if strings.HasPrefix(path, jaegerModule) {
			imports = append(imports, path)
		}
	}
	return imports
}
