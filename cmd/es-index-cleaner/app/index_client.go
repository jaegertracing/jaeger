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

package app

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

// Index represents ES index.
type Index struct {
	// Index name.
	Index string
	// Index creation time.
	CreationTime time.Time
	// Aliases
	Aliases map[string]bool
}

// IndicesClient is a client used to manipulate with indices.
type IndicesClient struct {
	// Http client.
	Client *http.Client
	// ES server endpoint.
	Endpoint string
	// ES master_timeout parameter.
	MasterTimeoutSeconds int
	BasicAuth            string
}

// GetJaegerIndices queries all Jaeger indices including the archive and rollover.
// Jaeger daily indices are:
//     jaeger-span-2019-01-01, jaeger-service-2019-01-01, jaeger-dependencies-2019-01-01
//     jaeger-span-archive
// Rollover indices:
//     aliases: jaeger-span-read, jaeger-span-write, jaeger-service-read, jaeger-service-write
//         indices: jaeger-span-000001, jaeger-service-000001 etc.
//     aliases: jaeger-span-archive-read, jaeger-span-archive-write
//         indices: jaeger-span-archive-000001
func (i *IndicesClient) GetJaegerIndices(prefix string) ([]Index, error) {
	prefix += "jaeger-*"
	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s?flat_settings=true&filter_path=*.aliases,*.settings", i.Endpoint, prefix), nil)
	if err != nil {
		return nil, err
	}
	i.setAuthorization(r)
	response, err := i.Client.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to query Jaeger indices: %q", err)
	}

	if response.StatusCode != 200 {
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("failed to query Jaeger indices: %s", string(body))
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	type indexInfo struct {
		Aliases  map[string]interface{} `json:"aliases"`
		Settings map[string]string      `json:"settings"`
	}
	var indicesInfo map[string]indexInfo
	if err = json.Unmarshal(body, &indicesInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshall response: %q", err)
	}

	var indices []Index
	for k, v := range indicesInfo {
		aliases := map[string]bool{}
		for alias := range v.Aliases {
			aliases[alias] = true
		}
		// ignoring error, ES should return valid date
		creationDate, _ := strconv.ParseInt(v.Settings["index.creation_date"], 10, 64)

		indices = append(indices, Index{
			Index:        k,
			CreationTime: time.Unix(0, int64(time.Millisecond)*creationDate),
			Aliases:      aliases,
		})
	}
	return indices, nil
}

// DeleteIndices deletes specified set of indices.
func (i *IndicesClient) DeleteIndices(indices []Index) error {
	concatIndices := ""
	for _, i := range indices {
		concatIndices += i.Index
		concatIndices += ","
	}

	r, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s?master_timeout=%ds", i.Endpoint, concatIndices, i.MasterTimeoutSeconds), nil)
	if err != nil {
		return err
	}
	i.setAuthorization(r)

	response, err := i.Client.Do(r)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		var body string
		if response.Body != nil {
			bodyBytes, err := ioutil.ReadAll(response.Body)
			if err != nil {
				return fmt.Errorf("deleting indices failed: %q", err)
			}
			body = string(bodyBytes)
		}
		return fmt.Errorf("deleting indices failed: %s", body)
	}
	return nil
}

func (i *IndicesClient) setAuthorization(r *http.Request) {
	if i.BasicAuth != "" {
		r.Header.Add("Authorization", fmt.Sprintf("Basic %s", i.BasicAuth))
	}
}
