// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestMainRejectsNegativeNumOfDays(t *testing.T) {
	const helperProcess = "JAEGER_TEST_NEGATIVE_RETENTION"
	if os.Getenv(helperProcess) == "1" {
		os.Args = []string{"jaeger-es-index-cleaner", "--", "-1", "http://localhost:0"}
		main()
		return
	}

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMainRejectsNegativeNumOfDays$")
	cmd.Env = append(os.Environ(), helperProcess+"=1")
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(output), "NUM_OF_DAYS argument must be non-negative")
}
