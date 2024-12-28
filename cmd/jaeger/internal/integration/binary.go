// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/ports"
)

// Binary is a wrapper around exec.Cmd to help running binaries in tests.
type Binary struct {
	Name            string
	HealthCheckPort int // overridable for Kafka tests which run two binaries and need different ports
	exec.Cmd
}

func (b *Binary) Start(t *testing.T) {
	outFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), "output_logs.txt"),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the %s output logs into %s", b.Name, outFile.Name())

	errFile, err := os.OpenFile(
		filepath.Join(t.TempDir(), "error_logs.txt"),
		os.O_CREATE|os.O_WRONLY,
		os.ModePerm,
	)
	require.NoError(t, err)
	t.Logf("Writing the %s error logs into %s", b.Name, errFile.Name())

	b.Stdout = outFile
	b.Stderr = errFile

	t.Logf("Running command: %v", b.Args)
	require.NoError(t, b.Cmd.Start())

	t.Cleanup(func() {
		if err := b.Process.Kill(); err != nil {
			t.Errorf("Failed to kill %s process: %v", b.Name, err)
		}
		t.Logf("Waiting for %s to exit", b.Name)
		b.Process.Wait()
		t.Logf("%s exited", b.Name)
		if t.Failed() {
			b.dumpLogs(t, outFile, errFile)
		}
	})

	// Wait for the binary to start and become ready to serve requests.
	require.Eventually(t, func() bool { return b.doHealthCheck(t) },
		60*time.Second, 3*time.Second, "%s did not start", b.Name)
	t.Logf("%s is ready", b.Name)
}

func (b *Binary) doHealthCheck(t *testing.T) bool {
	healthCheckPort := b.HealthCheckPort
	if healthCheckPort == 0 {
		healthCheckPort = ports.CollectorV2HealthChecks
	}
	healthCheckEndpoint := fmt.Sprintf("http://localhost:%d/status", healthCheckPort)
	t.Logf("Checking if %s is available on %s", b.Name, healthCheckEndpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthCheckEndpoint, nil)
	if err != nil {
		t.Logf("HTTP request creation failed: %v", err)
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("HTTP request failed: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Logf("Failed to read HTTP response body: %v", err)
		return false
	}
	if resp.StatusCode != http.StatusOK {
		t.Logf("HTTP response not OK: %v", string(body))
		return false
	}

	var healthResponse struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&healthResponse); err != nil {
		t.Logf("Failed to decode JSON response '%s': %v", string(body), err)
		return false
	}

	// Check if the status field in the JSON is "StatusOK"
	if healthResponse.Status != "StatusOK" {
		t.Logf("Received non-OK status %s: %s", healthResponse.Status, string(body))
		return false
	}
	return true
}

// Special Github markers to create a foldable section in the GitHub Actions runner output.
// https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions#grouping-log-lines
const (
	githubBeginGroup = "::group::"
	githubEndGroup   = "::endgroup::"
)

func (b *Binary) dumpLogs(t *testing.T, outFile, errFile *os.File) {
	fmt.Printf(githubBeginGroup+" 🚧 🚧 🚧 %s binary logs\n", b.Name)
	outLogs, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Errorf("Failed to read output logs: %v", err)
	} else {
		fmt.Printf("⏩⏩⏩ Start %s output logs:\n", b.Name)
		fmt.Printf("%s\n", outLogs)
		fmt.Printf("⏹️⏹️⏹️ End %s output logs.\n", b.Name)
	}

	errLogs, err := os.ReadFile(errFile.Name())
	if err != nil {
		t.Errorf("Failed to read error logs: %v", err)
	} else {
		fmt.Printf("⏩⏩⏩ Start %s error logs:\n", b.Name)
		fmt.Printf("%s\n", errLogs)
		fmt.Printf("⏹️⏹️⏹️ End %s error logs.\n", b.Name)
	}
	fmt.Println(githubEndGroup)
}
