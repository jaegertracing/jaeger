// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"

	esV8 "github.com/elastic/go-elasticsearch/v8"
	esV8api "github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

const (
	queryURL         = "http://0.0.0.0:9200"
	defaultIlmPolicy = "jaeger-default-ilm-policy"
	defaultIsmPolicy = "jaeger-default-ism-policy"
	prefix           = "jaeger-main-"
)

type E2EElasticSearchILMIntegration struct {
	isOpenSearch bool
}

func (e *E2EElasticSearchILMIntegration) RunTests(t *testing.T) {
	hcl := getESHttpClient(t)
	client, err := createESClient(t, hcl)
	require.NoError(t, err)
	defer e.cleanES(t, client)
	version, err := e.getVersion(client)
	require.NoError(t, err)
	if version < 7 {
		t.Skip("Automated Rollover is supported only for 7+ versions")
	}
	if e.isOpenSearch {
		s := &E2EStorageIntegration{
			ConfigFile: "../../config-opensearch-ilm.yaml",
			StorageIntegration: integration.StorageIntegration{
				CleanUp:  purge,
				Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			},
		}
		s.e2eInitialize(t, "opensearch")
	} else {
		s := &E2EStorageIntegration{
			ConfigFile: "../../config-elasticsearch-ilm.yaml",
			StorageIntegration: integration.StorageIntegration{
				CleanUp:  purge,
				Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			},
		}
		s.e2eInitialize(t, "elasticsearch")
	}
	var v8client *esV8.Client
	if version > 7 {
		v8client, err = createESV8Client(hcl.Transport)
		require.NoError(t, err)
	}
	t.Run("CheckForILMOrISMPolicy", func(t *testing.T) {
		e.checkForIlmPolicyCreation(t, client)
	})
	t.Run("CheckForCorrectTemplateSettings", func(t *testing.T) {
		e.checkForTemplateSettings(t, client, v8client, version)
	})
	t.Run("CheckForIndexAndAliasesExistence", func(t *testing.T) {
		e.checkForIndexAndAliasesExistence(t, client)
	})
}

func (e *E2EElasticSearchILMIntegration) checkForIlmPolicyCreation(t *testing.T, client *elastic.Client) {
	if !e.isOpenSearch {
		res, err := client.XPackIlmGetLifecycle().Policy(defaultIlmPolicy).Do(context.Background())
		require.NoError(t, err)
		if _, ok := res[defaultIlmPolicy]; !ok {
			t.Errorf("no policy found for %s", defaultIlmPolicy)
		}
	} else {
		_, err := client.PerformRequest(context.Background(), elastic.PerformRequestOptions{
			Path:   "/_plugins/_ism/policies/" + defaultIsmPolicy,
			Method: http.MethodGet,
		})
		require.NoError(t, err)
	}
}

func (e *E2EElasticSearchILMIntegration) checkForTemplateSettings(t *testing.T, client *elastic.Client, v8Client *esV8.Client, version uint) {
	templateNames := []string{"jaeger-span", "jaeger-service"}
	for _, templateName := range templateNames {
		t.Run(templateName, func(t *testing.T) {
			if version > 7 {
				flatSettings := true
				res, err := v8Client.Indices.GetIndexTemplate(func(request *esV8api.IndicesGetIndexTemplateRequest) {
					request.FlatSettings = &flatSettings
					request.Name = prefix + templateName
					request.Pretty = true
				})
				require.NoError(t, err)
				require.Equal(t, 200, res.StatusCode)
				data, err := io.ReadAll(res.Body)
				require.NoError(t, err)
				dataStr := string(data)
				// Parsing the whole template doesn't seems to be worth here as we are only concerned about correct policy application
				expectedLifecycleName := fmt.Sprintf("\"index.lifecycle.name\" : \"%s\"", defaultIlmPolicy)
				expectedRolloverAlias := fmt.Sprintf("\"index.lifecycle.rollover_alias\" : \"%s-write\"", prefix+templateName)
				assert.Contains(t, dataStr, expectedLifecycleName)
				assert.Contains(t, dataStr, expectedRolloverAlias)
			} else {
				tmpl, err := client.IndexGetTemplate().FlatSettings(true).Do(context.Background())
				require.NoError(t, err)
				template, ok := tmpl[prefix+templateName]
				require.True(t, ok)
				aliasName := prefix + templateName + "-" + "write"
				if !e.isOpenSearch {
					if lifecycleName, ok := template.Settings["index.lifecycle.name"]; ok {
						assert.Equal(t, defaultIlmPolicy, lifecycleName)
					} else {
						t.Errorf("no lifecycle name found for %s", defaultIlmPolicy)
					}
					if rolloverAlias, ok := template.Settings["index.lifecycle.rollover_alias"]; ok {
						assert.Equal(t, rolloverAlias, aliasName)
					} else {
						t.Errorf("no rollover_alias found for %s", defaultIlmPolicy)
					}
				} else {
					if rolloverAlias, ok := template.Settings["index.plugins.index_state_management.rollover_alias"]; ok {
						assert.Equal(t, rolloverAlias, aliasName)
					} else {
						t.Errorf("no rollover_alias found for %s", defaultIsmPolicy)
					}
				}
			}
		})
	}
}

func (*E2EElasticSearchILMIntegration) checkForIndexAndAliasesExistence(t *testing.T, client *elastic.Client) {
	indexes := []string{"jaeger-span", "jaeger-service"}
	suffix := "-000001"
	for _, indexName := range indexes {
		t.Run(indexName, func(t *testing.T) {
			index := prefix + indexName + suffix
			exists, err := client.IndexExists(index).Do(context.Background())
			require.NoError(t, err)
			require.True(t, exists)
			aliases, err := client.Aliases().Index(index).Do(context.Background())
			require.NoError(t, err)
			readIndexExists := false
			writeIndexExists := false
			if indexInfo, found := aliases.Indices[index]; found {
				for _, alias := range indexInfo.Aliases {
					if alias.AliasName == prefix+indexName+"-write" {
						writeIndexExists = true
						assert.True(t, alias.IsWriteIndex)
					} else if alias.AliasName == prefix+indexName+"-read" {
						readIndexExists = true
						assert.False(t, alias.IsWriteIndex)
					}
				}
			} else {
				t.Errorf("no alias found for %s", indexName)
			}
			assert.True(t, writeIndexExists)
			assert.True(t, readIndexExists)
		})
	}
}

func (e *E2EElasticSearchILMIntegration) cleanES(t *testing.T, client *elastic.Client) {
	_, err := client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
	if !e.isOpenSearch {
		_, err = client.XPackIlmDeleteLifecycle().Policy(defaultIlmPolicy).Do(context.Background())
		if err != nil && !elastic.IsNotFound(err) {
			assert.Fail(t, "Not able to clean up ILM Policy")
		}
	} else {
		_, err := client.PerformRequest(context.Background(), elastic.PerformRequestOptions{
			Path:   "/_plugins/_ism/policies/" + defaultIsmPolicy,
			Method: http.MethodDelete,
		})
		require.NoError(t, err)
	}
	_, err = client.IndexDeleteTemplate("*").Do(context.Background())
	require.NoError(t, err)
}

func (e *E2EElasticSearchILMIntegration) getVersion(client *elastic.Client) (uint, error) {
	if e.isOpenSearch {
		return 7, nil
	}
	pingResult, _, err := client.Ping(queryURL).Do(context.Background())
	if err != nil {
		return 0, err
	}
	esVersion, err := strconv.Atoi(string(pingResult.Version.Number[0]))
	if err != nil {
		return 0, err
	}
	//nolint: gosec
	return uint(esVersion), nil
}

func createESClient(t *testing.T, hcl *http.Client) (*elastic.Client, error) {
	cl, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false),
		elastic.SetHttpClient(hcl),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cl.Stop()
	})
	return cl, nil
}

func createESV8Client(tr http.RoundTripper) (*esV8.Client, error) {
	return esV8.NewClient(esV8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
		Transport:            tr,
	})
}

func getESHttpClient(t *testing.T) *http.Client {
	tr := &http.Transport{}
	t.Cleanup(func() {
		tr.CloseIdleConnections()
	})
	return &http.Client{Transport: tr}
}
