// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package env

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestCommand(t *testing.T) {
	cmd := Command()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.Run(cmd, nil)
	assert.Contains(t, buf.String(), "METRICS_BACKEND")
	assert.Contains(t, buf.String(), "SPAN_STORAGE")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
