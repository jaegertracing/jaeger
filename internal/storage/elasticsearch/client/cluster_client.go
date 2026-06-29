// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

var _ ClusterAPI = (*ClusterClient)(nil)

// ClusterClient is a client used to get ES cluster information
type ClusterClient struct {
	Client
}

// Version returns the detected backend version.
func (c *ClusterClient) Version(ctx context.Context) (es.BackendVersion, error) {
	type clusterInfo struct {
		Version map[string]any `json:"version"`
		TagLine string         `json:"tagline"`
	}
	body, err := c.request(ctx, elasticRequest{
		endpoint: "",
		method:   http.MethodGet,
	})
	if err != nil {
		return 0, err
	}
	var info clusterInfo
	err = json.Unmarshal(body, &info)
	if err != nil {
		return 0, err
	}

	versionField := info.Version["number"]
	versionNumber, isString := versionField.(string)
	if !isString {
		return 0, fmt.Errorf("invalid version format: %v", versionField)
	}
	version := strings.Split(versionNumber, ".")
	major, err := strconv.Atoi(version[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version format: %s", version[0])
	}
	return es.DetectBackendVersion(info.TagLine, major), nil
}

// IsOpenSearch reports whether the cluster is OpenSearch (as opposed to
// Elasticsearch), detected from the root endpoint's tagline. This selects the
// lifecycle management style for data streams: ISM on OpenSearch, ILM on
// Elasticsearch. See RFC 0004 section 3.8.
func (c *ClusterClient) IsOpenSearch(ctx context.Context) (bool, error) {
	type clusterInfo struct {
		TagLine string `json:"tagline"`
	}
	body, err := c.request(ctx, elasticRequest{
		endpoint: "",
		method:   http.MethodGet,
	})
	if err != nil {
		return false, err
	}
	var info clusterInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return false, err
	}
	return strings.Contains(info.TagLine, "OpenSearch"), nil
}
