// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var _ ClusterAPI = (*ClusterClient)(nil)

// ClusterClient is a client used to get ES cluster information
type ClusterClient struct {
	Client
}

// Version returns the major version of the ES cluster
func (c *ClusterClient) Version() (uint, error) {
	type clusterInfo struct {
		Version map[string]any `json:"version"`
		TagLine string         `json:"tagline"`
	}
	body, err := c.request(elasticRequest{
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
	major, err := strconv.ParseUint(version[0], 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid version format: %s", version[0])
	}
	if strings.Contains(info.TagLine, "OpenSearch") && (major == 1 || major == 2 || major == 3) {
		return 7, nil
	}
	return uint(major), nil
}

// IsOpenSearch reports whether the cluster is OpenSearch (as opposed to
// Elasticsearch), detected from the root endpoint's tagline. This selects the
// lifecycle management style for data streams: ISM on OpenSearch, ILM on
// Elasticsearch. See RFC 0004 section 3.8.
func (c *ClusterClient) IsOpenSearch() (bool, error) {
	type clusterInfo struct {
		TagLine string `json:"tagline"`
	}
	body, err := c.request(elasticRequest{
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
