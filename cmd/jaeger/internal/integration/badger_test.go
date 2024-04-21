// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

const (
	host = "0.0.0.0"
	Port = "9231"
	URL  = "/purge"
	Addr = "http://" + host + ":" + Port
)

type BadgerStorageIntegration struct {
	E2EStorageIntegration
	logger *zap.Logger
}

func (s *BadgerStorageIntegration) initialize(t *testing.T) {
	s.e2eInitialize(t)

	s.CleanUp = s.cleanUp
	s.logger = zap.NewNop()
}

func (s *BadgerStorageIntegration) cleanUp(t *testing.T) {
	r, err := http.NewRequest(http.MethodPost, Addr+URL, nil)
	require.NoError(t, err)

	client := &http.Client{}

	resp, err := client.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "badger")

	s := &BadgerStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: createBadgerCleanerConfig(t),
			StorageIntegration: integration.StorageIntegration{
				SkipBinaryAttrs: true,
				SkipArchiveTest: true,

				// TODO: remove this once badger supports returning spanKind from GetOperations
				// Cf https://github.com/jaegertracing/jaeger/issues/1922
				GetOperationsMissingSpanKind: true,
			},
		},
	}
	defer s.rmBadgerCleanerConfig(t)
	s.initialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunAll(t)
}

func createBadgerCleanerConfig(t *testing.T) string {
	cmd := exec.Command("../../../../scripts/prepare-badger-integration-tests.py")
	data, err := cmd.Output()
	require.NoError(t, err)
	tempFile := string(data)
	tempFile = strings.ReplaceAll(tempFile, "\n", "")
	return tempFile
}

func (s *BadgerStorageIntegration) rmBadgerCleanerConfig(t *testing.T) {
	err := os.Remove(s.ConfigFile)
	require.NoError(t, err)
}
