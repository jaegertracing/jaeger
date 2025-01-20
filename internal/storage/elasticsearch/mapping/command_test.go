// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mapping

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestCommandExecute(t *testing.T) {
	o := app.Options{
		Mapping:       "jaeger-span",
		EsVersion:     7,
		Shards:        5,
		Replicas:      1,
		IndexPrefix:   "jaeger-index",
		UseILM:        "false",
		ILMPolicyName: "jaeger-ilm-policy",
	}
	cmd := Command(o)

	oldStdout := os.Stdout
	defer func() {
		os.Stdout = oldStdout
	}()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})

	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	err = cmd.ParseFlags([]string{
		"--mapping=jaeger-span",
		"--es-version=7",
		"--shards=5",
		"--replicas=1",
		"--index-prefix=jaeger-index",
		"--use-ilm=false",
		"--ilm-policy-name=jaeger-ilm-policy",
	})
	require.NoError(t, err)
	require.NoError(t, cmd.Execute())

	_ = w.Close()
	<-done

	capturedOutput := buf.String()

	assert.True(t, strings.HasPrefix(capturedOutput, "{"), "Output should be JSON starting with '{'")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(capturedOutput), "}"), "Output should end with '}'")

	assert.Contains(t, capturedOutput, `"index_patterns":`, "Output should contain 'index_patterns'")
	assert.Contains(t, capturedOutput, `"index.number_of_shards":`, "Output should contain 'index.number_of_shards'")
	assert.Contains(t, capturedOutput, `"index.number_of_replicas":`, "Output should contain 'index.number_of_replicas'")
	assert.Contains(t, capturedOutput, `"mappings":`, "Output should contain 'mappings'")

	var jsonOutput map[string]any
	err = json.Unmarshal([]byte(capturedOutput), &jsonOutput)
	require.NoError(t, err, "Output should be valid JSON")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
