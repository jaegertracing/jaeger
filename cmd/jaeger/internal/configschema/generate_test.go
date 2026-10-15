// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/configschema/testdata"
)

func TestGenerate_TestConfig(t *testing.T) {
	cfgType := component.MustNewType("test")
	factory := extension.NewFactory(
		cfgType,
		func() component.Config { return testdata.DefaultTestConfig() },
		nil,
		component.StabilityLevelDevelopment,
	)
	factories := otelcol.Factories{
		Extensions: map[component.Type]extension.Factory{cfgType: factory},
	}

	g := newGenerator()
	testdataDir := filepath.Join(moduleRoot(), "cmd", "jaeger", "internal", "configschema", "testdata")
	require.NoError(t, g.docs.load(modulePath(), []string{testdataDir}))

	schema, err := g.generate(factories)
	require.NoError(t, err)

	actual, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	expected, err := os.ReadFile("testdata/testconfig_schema.json")
	require.NoError(t, err)

	assert.JSONEq(t, string(expected), string(actual))
}
