// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/ports"
)

// defaultShutdownTimeout is how long Stop waits for the process to exit after
// SIGTERM before escalating to SIGKILL.
const defaultShutdownTimeout = time.Minute

// Binary is a wrapper around exec.Cmd to help running binaries in tests.
type Binary struct {
	Name            string
	HealthCheckPort int // overridable for Kafka tests which run two binaries and need different ports
	// ShutdownTimeout overrides how long Stop waits after SIGTERM before
	// escalating to SIGKILL. Zero means defaultShutdownTimeout.
	ShutdownTimeout time.Duration
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
		b.Stop(t)
		// Dump logs if test failed, or if running in GitHub Actions where
		// t.Failed() may not return true when the test times out with a panic.
		if t.Failed() || os.Getenv("GITHUB_ACTIONS") == "true" {
			b.dumpLogs(t, outFile, errFile)
		}
		if err := outFile.Close(); err != nil {
			t.Logf("Failed to close output log file: %v", err)
		}
		if err := errFile.Close(); err != nil {
			t.Logf("Failed to close error log file: %v", err)
		}
	})
	client := testingHttpClient(t)
	// Wait for the binary to start and become ready to serve requests.
	require.Eventually(t, func() bool { return b.doHealthCheck(t, client) },
		time.Minute, time.Second, "%s did not start", b.Name)
	t.Logf("%s is ready", b.Name)
}

// Stop shuts the process down and waits for it to exit.
//
// SIGTERM is sent first so the binary runs its own shutdown path — for jaeger
// that means the collector's Shutdown hooks and storage Close calls, which
// SIGKILL skips entirely, hiding any bug in them from every e2e test. A kill is
// still a kill: if the process has not exited within ShutdownTimeout, it is
// escalated to SIGKILL, which cannot be caught or ignored, so Stop always
// terminates the process rather than trusting it to cooperate.
//
// It is safe to call more than once (and after the process has already exited):
// once the process has been reaped, signalling it returns os.ErrProcessDone,
// which is treated as a successful stop.
func (b *Binary) Stop(t *testing.T) {
	t.Helper()
	timeout := b.ShutdownTimeout
	if timeout == 0 {
		timeout = defaultShutdownTimeout
	}

	if err := b.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("Failed to signal %s process: %v", b.Name, err)
	}
	t.Logf("Waiting up to %v for %s to exit after SIGTERM", timeout, b.Name)

	// A single Wait runs in the background: os.Process.Wait must not be called
	// concurrently or twice, so the escalation path below waits on this channel
	// rather than calling Wait again.
	exited := make(chan struct{})
	go func() {
		b.Process.Wait()
		close(exited)
	}()

	select {
	case <-exited:
	case <-time.After(timeout):
		t.Logf("%s did not exit within %v of SIGTERM, sending SIGKILL", b.Name, timeout)
		if err := b.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("Failed to kill %s process: %v", b.Name, err)
		}
		<-exited
	}
	t.Logf("%s exited", b.Name)
}

func (b *Binary) doHealthCheck(t *testing.T, client *http.Client) bool {
	healthCheckPort := b.HealthCheckPort
	if healthCheckPort == 0 {
		healthCheckPort = ports.CollectorV2HealthChecks
	}
	healthCheckEndpoint := fmt.Sprintf("http://localhost:%d/status", healthCheckPort)
	t.Logf("Checking if %s is available on %s", b.Name, healthCheckEndpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthCheckEndpoint, http.NoBody)
	if err != nil {
		t.Logf("HTTP request creation failed: %v", err)
		return false
	}
	resp, err := client.Do(req)
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
