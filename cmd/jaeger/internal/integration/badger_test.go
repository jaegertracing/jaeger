// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/storagecleaner"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func cleanUp(t *testing.T) {
	Addr := fmt.Sprintf("http://0.0.0.0:%s%s", storagecleaner.Port, storagecleaner.URL)
	r, err := http.NewRequest(http.MethodPost, Addr, nil)
	require.NoError(t, err)

	client := &http.Client{}

	resp, err := client.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "badger")

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-badger.yaml",
		StorageIntegration: integration.StorageIntegration{
			SkipBinaryAttrs: true,
			SkipArchiveTest: true,
			CleanUp:         cleanUp,

			// TODO: remove this once badger supports returning spanKind from GetOperations
			// Cf https://github.com/jaegertracing/jaeger/issues/1922
			GetOperationsMissingSpanKind: true,
		},
	}
	s.e2eInitialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunAll(t)
}
