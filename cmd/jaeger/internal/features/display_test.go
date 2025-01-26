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
		"jaeger.featuresdisplay.testgate",
		featuregate.StageBeta,
		featuregate.WithRegisterDescription("test-description"),
		featuregate.WithRegisterReferenceURL("https://test-url.com"),
	)
	out := DisplayFeatures()
	require.Contains(t, out, "jaeger.featuresdisplay.testgate")
}
