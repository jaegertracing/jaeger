// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func readOTLPTraces(t *testing.T) []byte {
	dat, err := os.ReadFile("./fixture/otlp2jaeger-in.json")
	require.NoError(t, err)
	return dat
}

func readJaegerTraces(t *testing.T) []byte {
	dat, err := os.ReadFile("./fixture/otlp2jaeger-out.json")
	require.NoError(t, err)
	return dat
}

func TestOtlp2Traces(t *testing.T) {
	OTLPTraces := readOTLPTraces(t)
	_, err := otlp2traces(OTLPTraces)
	require.NoError(t, err)
	// Correctness of outputs wrt to fixtures are tested while testing http endpoints
}
