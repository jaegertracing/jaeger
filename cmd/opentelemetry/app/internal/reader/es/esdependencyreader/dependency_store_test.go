// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package esdependencyreader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore/dbmodel"
)

const defaultMaxDocCount = 10_000

func TestCreateTemplates(t *testing.T) {
	client := &mockClient{}
	store := NewDependencyStore(client, zap.NewNop(), "foo", "2006-01-02", defaultMaxDocCount)
	template := "template"
	err := store.CreateTemplates(template)
	require.NoError(t, err)
	receivedBody, err := ioutil.ReadAll(client.receivedBody)
	require.NoError(t, err)
	assert.Equal(t, template, string(receivedBody))
}

func TestWriteDependencies(t *testing.T) {
	client := &mockClient{}
	store := NewDependencyStore(client, zap.NewNop(), "foo", "2006-01-02", defaultMaxDocCount)
	dependencies := []model.DependencyLink{{Parent: "foo", Child: "bar", CallCount: 1}}
	tsNow := time.Now()
	err := store.WriteDependencies(tsNow, dependencies)
	require.NoError(t, err)

	d := &dbmodel.TimeDependencies{
		Timestamp:    tsNow,
		Dependencies: dbmodel.FromDomainDependencies(dependencies),
	}
	jsonDependencies, err := json.Marshal(d)
	require.NoError(t, err)

	receivedBody, err := ioutil.ReadAll(client.receivedBody)
	require.NoError(t, err)
	assert.Equal(t, jsonDependencies, receivedBody)
}

func TestGetDependencies(t *testing.T) {
	tsNow := time.Now()
	timeDependencies := dbmodel.TimeDependencies{
		Timestamp: tsNow,
		Dependencies: []dbmodel.DependencyLink{
			{Parent: "foo", Child: "bar"},
		},
	}
	jsonDep, err := json.Marshal(timeDependencies)
	require.NoError(t, err)
	rawMessage := json.RawMessage(jsonDep)
	client := &mockClient{
		searchResponse: &esclient.SearchResponse{
			Hits: esclient.Hits{
				Total: 1,
				Hits: []esclient.Hit{
					{Source: &rawMessage},
				},
			},
		},
	}
	store := NewDependencyStore(client, zap.NewNop(), "foo", "2006-01-02", defaultMaxDocCount)
	dependencies, err := store.GetDependencies(context.Background(), tsNow, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, timeDependencies, dbmodel.TimeDependencies{
		Timestamp:    tsNow,
		Dependencies: dbmodel.FromDomainDependencies(dependencies),
	})
}

func TestGetDependencies_err_unmarshall(t *testing.T) {
	tsNow := time.Now()
	rawMessage := json.RawMessage("#")
	client := &mockClient{
		searchResponse: &esclient.SearchResponse{
			Hits: esclient.Hits{
				Total: 1,
				Hits: []esclient.Hit{
					{Source: &rawMessage},
				},
			},
		},
	}
	store := NewDependencyStore(client, zap.NewNop(), "foo", "2006-01-02", defaultMaxDocCount)
	dependencies, err := store.GetDependencies(context.Background(), tsNow, time.Hour)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid character")
	assert.Nil(t, dependencies)
}

func TestGetDependencies_err_client(t *testing.T) {
	searchErr := fmt.Errorf("client err")
	client := &mockClient{
		searchErr: searchErr,
	}
	store := NewDependencyStore(client, zap.NewNop(), "foo", "2006-01-02", defaultMaxDocCount)
	tsNow := time.Now()
	dependencies, err := store.GetDependencies(context.Background(), tsNow, time.Hour)
	require.Error(t, err)
	assert.Nil(t, dependencies)
	assert.Contains(t, err.Error(), searchErr.Error())
}

const query = `{
  "query": {
    "range": {
      "timestamp": {
        "gte": "2020-08-30T14:00:00Z",
        "lte": "2020-08-30T15:00:00Z"
      }
    }
  },
  "size": 10000,
  "terminate_after": 0
}`

func TestSearchBody(t *testing.T) {
	date := time.Date(2020, 8, 30, 15, 0, 0, 0, time.UTC)
	sb := getSearchBody(date, time.Hour, defaultMaxDocCount)
	jsonQuery, err := json.MarshalIndent(sb, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, query, string(jsonQuery))
}

func TestIndexWithDate(t *testing.T) {
	assert.Equal(t, "foo-2020-09-30", indexWithDate("foo-", "2006-01-02",
		time.Date(2020, 9, 30, 0, 0, 0, 0, time.UTC)))
}

func TestDailyIndices(t *testing.T) {
	indices := dailyIndices("foo-", "2006-01-02", time.Date(2020, 9, 30, 0, 0, 0, 0, time.UTC), time.Hour)
	assert.Equal(t, []string{"foo-2020-09-30", "foo-2020-09-29"}, indices)
}

type mockClient struct {
	receivedBody   io.Reader
	searchResponse *esclient.SearchResponse
	searchErr      error
}

var _ esclient.ElasticsearchClient = (*mockClient)(nil)

func (m *mockClient) PutTemplate(ctx context.Context, name string, template io.Reader) error {
	m.receivedBody = template
	return nil
}

func (m mockClient) Bulk(ctx context.Context, bulkBody io.Reader) (*esclient.BulkResponse, error) {
	panic("implement me")
}

func (m mockClient) AddDataToBulkBuffer(bulkBody *bytes.Buffer, data []byte, index, typ string) {
	panic("implement me")
}

func (m *mockClient) Index(ctx context.Context, body io.Reader, index, typ string) error {
	m.receivedBody = body
	return nil
}

func (m *mockClient) Search(ctx context.Context, query esclient.SearchBody, size int, indices ...string) (*esclient.SearchResponse, error) {
	return m.searchResponse, m.searchErr
}

func (m mockClient) MultiSearch(ctx context.Context, queries []esclient.SearchBody) (*esclient.MultiSearchResponse, error) {
	panic("implement me")
}

func (m *mockClient) MajorVersion() int {
	panic("implement me")
}
