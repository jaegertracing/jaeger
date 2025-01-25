// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package features

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestDisplay(t *testing.T) {
	internal.Command()
	out := DisplayFeatures()
	parts := strings.Split(out, "\n\n")
	parts = parts[:len(parts)-1]
	feature_re := regexp.MustCompile(`(?m)Feature:\s*(.*)`)
	description_re := regexp.MustCompile(`(?m)Description:\s*(.*)`)
	for _, part := range parts {
		feature_matches := feature_re.FindStringSubmatch(part)
		require.Greater(t, len(feature_matches), 1)
		description_matches := description_re.FindStringSubmatch(part)
		require.Greater(t, len(description_matches), 1)
	}
}
