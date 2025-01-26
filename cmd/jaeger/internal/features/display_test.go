// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package features

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestDisplay(t *testing.T) {
	featuregate.GlobalRegistry().MustRegister(
		"jaeger.sampling.includeDefaultOpStrategies",
		featuregate.StageBeta, // enabed by default
		featuregate.WithRegisterFromVersion("v2.2.0"),
		featuregate.WithRegisterToVersion("v2.5.0"),
		featuregate.WithRegisterDescription("Forces service strategy to be merged with default strategy, including per-operation overrides."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/5270"),
	)
	out := DisplayFeatures()
	expected_out := "Feature:\tjaeger.sampling.includeDefaultOpStrategies\nDefault state:\tOn\nDescription:\tForces service strategy to be merged with default strategy, including per-operation overrides.\nReferenceURL:\thttps://github.com/jaegertracing/jaeger/issues/5270\n\n"
	require.Equal(t, expected_out, out)
}
