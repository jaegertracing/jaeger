// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

var _ ClusterAPI = (*ClusterClient)(nil)

// ClusterClient is a client used to get ES cluster information
type ClusterClient struct {
	Client
}

// ping fetches the raw version fields from the cluster root document ("GET /").
// Version derivation is left to es.ResolveBackendVersion so that both client
// planes share one detection path.
func (c *ClusterClient) ping(ctx context.Context) (es.PingResult, error) {
	type clusterInfo struct {
		Version map[string]any `json:"version"`
		TagLine string         `json:"tagline"`
	}
	body, err := c.request(ctx, elasticRequest{
		endpoint: "",
		method:   http.MethodGet,
	})
	if err != nil {
		return es.PingResult{}, err
	}
	var info clusterInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return es.PingResult{}, err
	}

	versionField := info.Version["number"]
	versionNumber, isString := versionField.(string)
	if !isString {
		return es.PingResult{}, fmt.Errorf("invalid version format: %v", versionField)
	}
	return es.PingResult{VersionNumber: versionNumber, TagLine: info.TagLine}, nil
}

// ResolveVersion returns the configured version when non-zero; otherwise it
// probes the cluster once. It shares es.ResolveBackendVersion with the
// data-plane builder so version detection has a single implementation.
func (c *ClusterClient) ResolveVersion(ctx context.Context, configured uint) (es.BackendVersion, error) {
	return es.ResolveBackendVersion(ctx, configured, c.ping)
}
