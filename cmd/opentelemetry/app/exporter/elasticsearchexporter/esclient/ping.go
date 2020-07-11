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

package esclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// elasticsearchPing is used to get Elasticsearch version.
// ES native client cannot be used because its version should be known beforehand.
type elasticsearchPing struct {
	username     string
	password     string
	roundTripper http.RoundTripper
}

func (p *elasticsearchPing) getPingResponse(url string) (*pingResponse, error) {
	client := http.Client{
		Transport: p.roundTripper,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if p.username != "" && p.password != "" {
		req.Header.Add("Authorization", "Basic "+basicAuth(p.username, p.password))
		client.CheckRedirect = redirectPolicyFunc(p.username, p.password)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var pingResp pingResponse
	if err := json.NewDecoder(response.Body).Decode(&pingResp); err != nil {
		return nil, err
	}
	return &pingResp, nil
}

func (p *elasticsearchPing) getVersion(url string) (int, error) {
	pingResponse, err := p.getPingResponse(url)
	if err != nil {
		return 0, err
	}
	esVersion, err := strconv.Atoi(string(pingResponse.Version.Number[0]))
	if err != nil {
		return 0, fmt.Errorf("elasticsearch verision %s cannot be converted to a number: %v", pingResponse.Version, err)
	}
	return esVersion, nil
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func redirectPolicyFunc(username, password string) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		auth := basicAuth(username, password)
		req.Header.Add("Authorization", "Basic "+auth)
		return nil
	}
}

type pingResponse struct {
	Name        string `json:"name"`
	ClusterName string `json:"cluster_name"`
	ClusterUUID string `json:"cluster_uuid"`
	Version     struct {
		Number string `json:"number"`
	} `json:"version"`
}
