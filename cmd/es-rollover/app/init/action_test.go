// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient/mocks"
)

func applyTestDefaults(cfg *Config) {
	indices := []*config.IndexOptions{
		&cfg.Indices.Spans,
		&cfg.Indices.Services,
		&cfg.Indices.Dependencies,
		&cfg.Indices.Sampling,
	}
	for _, idx := range indices {
		if idx.Shards == 0 {
			idx.Shards = 3
		}
		if idx.Replicas == nil {
			idx.Replicas = new(int64(1))
		}
		if idx.Priority == 0 {
			idx.Priority = 10
		}
	}
}

func TestIndexCreateIfNotExist(t *testing.T) {
	tests := []struct {
		name           string
		indexExists    bool
		indexExistsErr error
		aliasExists    bool
		aliasExistsErr error
		createIndexErr error
		expectedError  string
	}{
		{
			name:        "success when index exists",
			indexExists: true,
		},
		{
			name:           "generic error from IndexExists",
			indexExistsErr: errors.New("may be an http error from index exists"),
			expectedError:  "may be an http error from index exists",
		},
		{
			name:        "success when alias exists",
			aliasExists: true,
		},
		{
			name:           "generic error from AliasExists",
			aliasExistsErr: errors.New("may be an http error from alias exists"),
			expectedError:  "may be an http error from alias exists",
		},
		{
			name:           "generic error from create index",
			createIndexErr: errors.New("may be an http error from create index"),
			expectedError:  "may be an http error from create index",
		},
		{
			name: "success when index and alias does not exist",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indexClient := &mocks.IndexAPI{}
			indexClient.On("IndexExists", mock.Anything, "jaeger-span").Return(test.indexExists, test.indexExistsErr)
			indexClient.On("AliasExists", mock.Anything, "jaeger-span").Return(test.aliasExists, test.aliasExistsErr)
			indexClient.On("CreateIndex", mock.Anything, "jaeger-span").Return(test.createIndexErr)
			err := createIndexIfNotExist(context.Background(), indexClient, "jaeger-span")
			if test.expectedError != "" {
				assert.EqualError(t, err, test.expectedError)
			}
		})
	}
}

func TestRolloverAction(t *testing.T) {
	tests := []struct {
		name                  string
		version               es.BackendVersion
		setupCallExpectations func(indexClient *mocks.IndexAPI, ilmClient *mocks.IndexManagementLifecycleAPI)
		config                Config
		expectedErr           error
	}{
		{
			name:                  "Unsupported version",
			version:               es.ElasticV6,
			setupCallExpectations: func(_ *mocks.IndexAPI, _ *mocks.IndexManagementLifecycleAPI) {},
			config: Config{
				Config: app.Config{
					Archive: true,
					UseILM:  true,
				},
			},
			expectedErr: errors.New("ILM/ISM is not supported by the Elasticsearch/OpenSearch backend"),
		},
		{
			name:    "ilm doesnt exist",
			version: es.ElasticV7,
			setupCallExpectations: func(_ *mocks.IndexAPI, ilmClient *mocks.IndexManagementLifecycleAPI) {
				ilmClient.On("Exists", mock.Anything, "myilmpolicy").Return(false, nil)
			},
			expectedErr: errors.New("ILM policy myilmpolicy doesn't exist in Elasticsearch. Please create it and re-run init"),
			config: Config{
				Config: app.Config{
					Archive:       true,
					UseILM:        true,
					ILMPolicyName: "myilmpolicy",
				},
			},
		},
		{
			name:    "fail get ilm policy",
			version: es.ElasticV7,
			setupCallExpectations: func(_ *mocks.IndexAPI, ilmClient *mocks.IndexManagementLifecycleAPI) {
				ilmClient.On("Exists", mock.Anything, "myilmpolicy").Return(false, errors.New("error getting ilm policy"))
			},
			expectedErr: errors.New("error getting ilm policy"),
			config: Config{
				Config: app.Config{
					Archive:       true,
					UseILM:        true,
					ILMPolicyName: "myilmpolicy",
				},
			},
		},
		{
			name:    "fail to create template",
			version: es.ElasticV7,
			setupCallExpectations: func(indexClient *mocks.IndexAPI, _ *mocks.IndexManagementLifecycleAPI) {
				indexClient.On("CreateTemplate", mock.Anything, "jaeger-span", mock.Anything).Return(errors.New("error creating template"))
			},
			expectedErr: errors.New("error creating template"),
			config: Config{
				Config: app.Config{
					Archive: true,
					UseILM:  false,
				},
			},
		},
		{
			name:    "fail to get jaeger indices",
			version: es.ElasticV7,
			setupCallExpectations: func(indexClient *mocks.IndexAPI, _ *mocks.IndexManagementLifecycleAPI) {
				indexClient.On("IndexExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("AliasExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("CreateTemplate", mock.Anything, "jaeger-span", mock.Anything).Return(nil)
				indexClient.On("CreateIndex", mock.Anything, "jaeger-span-archive-000001").Return(nil)
				indexClient.On("GetJaegerIndices", mock.Anything, "").Return([]esclient.Index{}, errors.New("error getting jaeger indices"))
			},
			expectedErr: errors.New("error getting jaeger indices"),
			config: Config{
				Config: app.Config{
					Archive: true,
					UseILM:  false,
				},
			},
		},
		{
			name:    "fail to create alias",
			version: es.ElasticV7,
			setupCallExpectations: func(indexClient *mocks.IndexAPI, _ *mocks.IndexManagementLifecycleAPI) {
				indexClient.On("IndexExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("AliasExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("CreateTemplate", mock.Anything, "jaeger-span", mock.Anything).Return(nil)
				indexClient.On("CreateIndex", mock.Anything, "jaeger-span-archive-000001").Return(nil)
				indexClient.On("GetJaegerIndices", mock.Anything, "").Return([]esclient.Index{}, nil)
				indexClient.On("CreateAlias", mock.Anything, []esclient.Alias{
					{Index: "jaeger-span-archive-000001", Name: "jaeger-span-archive-read", IsWriteIndex: false},
					{Index: "jaeger-span-archive-000001", Name: "jaeger-span-archive-write", IsWriteIndex: false},
				}).Return(errors.New("error creating aliases"))
			},
			expectedErr: errors.New("error creating aliases"),
			config: Config{
				Config: app.Config{
					Archive: true,
					UseILM:  false,
				},
			},
		},
		{
			name:    "create rollover index",
			version: es.ElasticV7,
			setupCallExpectations: func(indexClient *mocks.IndexAPI, _ *mocks.IndexManagementLifecycleAPI) {
				indexClient.On("IndexExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("AliasExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("CreateTemplate", mock.Anything, "jaeger-span", mock.Anything).Return(nil)
				indexClient.On("CreateIndex", mock.Anything, "jaeger-span-archive-000001").Return(nil)
				indexClient.On("GetJaegerIndices", mock.Anything, "").Return([]esclient.Index{}, nil)
				indexClient.On("CreateAlias", mock.Anything, []esclient.Alias{
					{Index: "jaeger-span-archive-000001", Name: "jaeger-span-archive-read", IsWriteIndex: false},
					{Index: "jaeger-span-archive-000001", Name: "jaeger-span-archive-write", IsWriteIndex: false},
				}).Return(nil)
			},
			expectedErr: nil,
			config: Config{
				Config: app.Config{
					Archive: true,
					UseILM:  false,
				},
			},
		},
		{
			name:    "create rollover index with ilm",
			version: es.ElasticV7,
			setupCallExpectations: func(indexClient *mocks.IndexAPI, ilmClient *mocks.IndexManagementLifecycleAPI) {
				indexClient.On("IndexExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("AliasExists", mock.Anything, "jaeger-span-archive-000001").Return(false, nil)
				indexClient.On("CreateTemplate", mock.Anything, "jaeger-span", mock.Anything).Return(nil)
				indexClient.On("CreateIndex", mock.Anything, "jaeger-span-archive-000001").Return(nil)
				indexClient.On("GetJaegerIndices", mock.Anything, "").Return([]esclient.Index{}, nil)
				ilmClient.On("Exists", mock.Anything, "jaeger-ilm").Return(true, nil)
				indexClient.On("CreateAlias", mock.Anything, []esclient.Alias{
					{Index: "jaeger-span-archive-000001", Name: "jaeger-span-archive-read", IsWriteIndex: false},
					{Index: "jaeger-span-archive-000001", Name: "jaeger-span-archive-write", IsWriteIndex: true},
				}).Return(nil)
			},
			expectedErr: nil,
			config: Config{
				Config: app.Config{
					Archive:       true,
					UseILM:        true,
					ILMPolicyName: "jaeger-ilm",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Apply local test defaults
			applyTestDefaults(&test.config)
			indexClient := &mocks.IndexAPI{}
			ilmClient := &mocks.IndexManagementLifecycleAPI{}
			if test.config.Config.UseILM {
				// The action gates on the client's ILM capability, not a version.
				ilmClient.On("SupportsILM").Return(test.version.SupportsILM())
			}
			initAction := Action{
				Config:        test.config,
				IndicesClient: indexClient,
				ILMClient:     ilmClient,
			}

			test.setupCallExpectations(indexClient, ilmClient)

			err := initAction.Do()
			if test.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, test.expectedErr, err)
			}

			indexClient.AssertExpectations(t)
			ilmClient.AssertExpectations(t)
		})
	}
}

func TestRolloverAction_OpenSearchUsesISMEndpoint(t *testing.T) {
	// Verify that when the backend is OpenSearch, the concrete ILMClient
	// selects the ISM endpoint from its injected version (no version handled
	// by the init action).
	var ismEndpointCalled atomic.Bool
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if strings.Contains(req.URL.String(), "_plugins/_ism/policies/") {
			ismEndpointCalled.Store(true)
		}
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	esClient, err := esclient.NewClient(
		context.Background(),
		&config.Configuration{Servers: []string{testServer.URL}},
		zap.NewNop(),
		nil,
	)
	require.NoError(t, err)
	esClient = esClient.WithVersion(es.OpenSearch2)

	ilmClient := &esclient.ILMClient{
		Client: esClient,
		Logger: zap.NewNop(),
	}

	cfg := Config{Config: app.Config{UseILM: true, ILMPolicyName: "test-policy"}}
	applyTestDefaults(&cfg)

	indexClient := &mocks.IndexAPI{}
	indexClient.On("CreateTemplate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	indexClient.On("IndexExists", mock.Anything, mock.Anything).Return(true, nil)
	indexClient.On("GetJaegerIndices", mock.Anything, "").Return([]esclient.Index{}, nil)
	indexClient.On("CreateAlias", mock.Anything, mock.Anything).Return(nil)

	action := Action{
		Config:        cfg,
		IndicesClient: indexClient,
		ILMClient:     ilmClient,
	}

	err = action.Do()
	require.NoError(t, err)
	assert.True(t, ismEndpointCalled.Load(), "expected ISM endpoint to be called")
}
