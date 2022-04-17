// Copyright (c) 2021 The Jaeger Authors.
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
		Version map[string]interface{} `json:"version"`
		TagLine string                 `json:"tagline"`
	}
	body, err := c.request(elasticRequest{
		endpoint: "/",
		method:   http.MethodGet,
	})

	if err != nil {
		return 0, err
	}
	var info clusterInfo
	if err = json.Unmarshal(body, &info); err != nil {
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
	if strings.Contains(info.TagLine, "OpenSearch") && major == 1 {
		return 7, nil
	}
	return uint(major), nil
}
