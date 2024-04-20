// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"net/http"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

const (
	host        = "0.0.0.0"
	cleanerPort = "9231"
	cleanerURL  = "/purge"
	cleanerAddr = "http://" + host + ":" + cleanerPort
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
	r, err := http.NewRequest(http.MethodPost, cleanerAddr+cleanerURL, nil)
	require.NoError(t, err)

	client := &http.Client{}

	resp, err := client.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "badger")

	s := &BadgerStorageIntegration{
		E2EStorageIntegration: E2EStorageIntegration{
			ConfigFile: "cmd/jaeger/badger_config.yaml",
			StorageIntegration: integration.StorageIntegration{
				SkipBinaryAttrs: true,
				SkipArchiveTest: true,

				// TODO: remove this once badger supports returning spanKind from GetOperations
				// Cf https://github.com/jaegertracing/jaeger/issues/1922
				GetOperationsMissingSpanKind: true,
			},
		},
	}
	s.addBadgerCleanerConfig(t)
	s.initialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunAll(t)
}

func (s *BadgerStorageIntegration) addBadgerCleanerConfig(t *testing.T) {
	cmd := exec.Command("../../../../scripts/prepare-badger-integration-tests.py")
	err := cmd.Run()
	require.NoError(t, err)
}
