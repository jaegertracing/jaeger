// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func healthCheck(c *http.Client) error {
	for i := 0; i < 200; i++ {
		if resp, err := c.Get(queryURL); err == nil {
			return resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("elastic search is not ready")
}

func testElasticsearchStorage(t *testing.T, allTagsAsFields bool) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{
		StorageIntegration: StorageIntegration{
			Fixtures:        LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			SkipArchiveTest: false,
			// TODO: remove this flag after ES supports returning spanKind
			//  Issue https://github.com/jaegertracing/jaeger/issues/1923
			GetOperationsMissingSpanKind: true,
		},
	}
	s.initializeES(t, c, allTagsAsFields)
	s.RunAll(t)
}

func TestElasticsearchStorage(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	testElasticsearchStorage(t, false)
}

func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	testElasticsearchStorage(t, true)
}

func TestElasticsearchStorage_IndexTemplates(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{}
	s.initializeES(t, c, true)
	esVersion, err := s.getVersion()
	require.NoError(t, err)
	// TODO abstract this into pkg/es/client.IndexManagementLifecycleAPI
	if esVersion == 6 || esVersion == 7 {
		serviceTemplateExists, err := s.client.IndexTemplateExists(indexPrefix + "-jaeger-service").Do(context.Background())
		require.NoError(t, err)
		assert.True(t, serviceTemplateExists)
		spanTemplateExists, err := s.client.IndexTemplateExists(indexPrefix + "-jaeger-span").Do(context.Background())
		require.NoError(t, err)
		assert.True(t, spanTemplateExists)
	} else {
		serviceTemplateExistsResponse, err := s.v8Client.API.Indices.ExistsIndexTemplate(indexPrefix + "-jaeger-service")
		require.NoError(t, err)
		assert.Equal(t, 200, serviceTemplateExistsResponse.StatusCode)
		spanTemplateExistsResponse, err := s.v8Client.API.Indices.ExistsIndexTemplate(indexPrefix + "-jaeger-span")
		require.NoError(t, err)
		assert.Equal(t, 200, spanTemplateExistsResponse.StatusCode)
	}
	s.cleanESIndexTemplates(t, indexPrefix)
}
