// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/require"
)

type EsClient struct {
	client   *elastic.Client
	v8Client *elasticsearch8.Client
}

func StartEsClient(t *testing.T, queryURL string) *EsClient {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false))
	require.NoError(t, err)

	rawV8Client, err := elasticsearch8.NewClient(elasticsearch8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
	})
	require.NoError(t, err)
	return &EsClient{
		client:   rawClient,
		v8Client: rawV8Client,
	}
}

func (s *EsClient) EsClientCleanup(t *testing.T) {
	_, err := s.client.DeleteIndex("*").Do(context.Background())
	require.NoError(t, err)
}

func (s *EsClient) EsClientRefresh(t *testing.T) {
	_, err := s.client.Refresh().Do(context.Background())
	require.NoError(t, err)
}
