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

// ping fetches the raw version fields from the cluster root document ("GET /").
// It is a low-level transport operation on the Client; version derivation is
// left to es.ResolveBackendVersion so both client planes share one detection
// path. NewClient calls it once at construction.
func (c Client) ping(ctx context.Context) (es.PingResult, error) {
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
