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

// Alias represents ES alias.
type Alias struct {
	// Index name.
	Index string
	// Alias name.
	Name string
	// IsWritedIndex option
	IsWriteIndex bool
}

var _ IndexAPI = (*IndicesClient)(nil)

// IndicesClient is a client used to manipulate indices.
type IndicesClient struct {
	Client
	MasterTimeoutSeconds int
}

// GetJaegerIndices queries all Jaeger indices including the archive and rollover.
// Jaeger daily indices are:
// - jaeger-span-2019-01-01
// - jaeger-service-2019-01-01
// - jaeger-dependencies-2019-01-01
// - jaeger-span-archive
//
// Rollover indices:
// - aliases: jaeger-span-read, jaeger-span-write, jaeger-service-read, jaeger-service-write
// - indices: jaeger-span-000001, jaeger-service-000001 etc.
// - aliases: jaeger-span-archive-read, jaeger-span-archive-write
// - indices: jaeger-span-archive-000001
func (i *IndicesClient) GetJaegerIndices(prefix string) ([]Index, error) {
	prefix += "jaeger-*"

	body, err := i.request(elasticRequest{
		endpoint: fmt.Sprintf("%s?flat_settings=true&filter_path=*.aliases,*.settings", prefix),
		method:   http.MethodGet,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query indices: %w", err)
	}

	type indexInfo struct {
		Aliases  map[string]interface{} `json:"aliases"`
		Settings map[string]string      `json:"settings"`
	}
	var indicesInfo map[string]indexInfo
	if err = json.Unmarshal(body, &indicesInfo); err != nil {
		return nil, fmt.Errorf("failed to query indices and unmarshall response body: %q: %w", body, err)
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

// execute delete request
func (i *IndicesClient) indexDeleteRequest(concatIndices string) error {
	_, err := i.request(elasticRequest{
		endpoint: fmt.Sprintf("%s?master_timeout=%ds", concatIndices, i.MasterTimeoutSeconds),
		method:   http.MethodDelete,
	})
	if err != nil {
		if responseError, isResponseError := err.(ResponseError); isResponseError {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage(fmt.Sprintf("failed to delete indices: %s", concatIndices))
			}
		}
		return fmt.Errorf("failed to delete indices: %w", err)
	}
	return nil
}

// DeleteIndices deletes specified set of indices.
func (i *IndicesClient) DeleteIndices(indices []Index) error {
	concatIndices := ""
	for j, index := range indices {
		// verify the length of the concatIndices
		// An HTTP line is should not be larger than 4096 bytes
		// a line contains other than concatIndices data in the request, ie: master_timeout
		// for a safer side check the line length should not exceed 4000
		if (len(concatIndices) + len(index.Index)) > 4000 {
			err := i.indexDeleteRequest(concatIndices)
			if err != nil {
				return err
			}
			concatIndices = ""
		}

		concatIndices += index.Index
		concatIndices += ","

		// if it is last index, delete request should be executed
		if j == len(indices)-1 {
			return i.indexDeleteRequest(concatIndices)
		}
	}
	return nil
}

// CreateIndex an ES index
func (i *IndicesClient) CreateIndex(index string) error {
	_, err := i.request(elasticRequest{
		endpoint: index,
		method:   http.MethodPut,
	})
	if err != nil {
		if responseError, isResponseError := err.(ResponseError); isResponseError {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage(fmt.Sprintf("failed to create index: %s", index))
			}
		}
		return fmt.Errorf("failed to create index: %w", err)
	}
	return nil
}

// CreateAlias an ES specific set of index aliases
func (i *IndicesClient) CreateAlias(aliases []Alias) error {
	err := i.aliasAction("add", aliases)
	if err != nil {
		if responseError, isResponseError := err.(ResponseError); isResponseError {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage(fmt.Sprintf("failed to create aliases: %s", i.aliasesString(aliases)))
			}
		}
		return fmt.Errorf("failed to create aliases: %w", err)
	}
	return nil
}

// DeleteAlias an ES specific set of index aliases
func (i *IndicesClient) DeleteAlias(aliases []Alias) error {
	err := i.aliasAction("remove", aliases)
	if err != nil {
		if responseError, isResponseError := err.(ResponseError); isResponseError {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage(fmt.Sprintf("failed to delete aliases: %s", i.aliasesString(aliases)))
			}
		}
		return fmt.Errorf("failed to delete aliases: %w", err)
	}
	return nil
}

func (i *IndicesClient) aliasesString(aliases []Alias) string {
	concatAliases := ""
	for _, alias := range aliases {
		concatAliases += fmt.Sprintf("[index: %s, alias: %s],", alias.Index, alias.Name)
	}
	return strings.Trim(concatAliases, ",")
}

func (i *IndicesClient) aliasAction(action string, aliases []Alias) error {
	actions := []map[string]interface{}{}

	for _, alias := range aliases {
		options := map[string]interface{}{
			"index": alias.Index,
			"alias": alias.Name,
		}
		if alias.IsWriteIndex {
			options["is_write_index"] = true
		}
		actions = append(actions, map[string]interface{}{
			action: options,
		})
	}

	body := map[string]interface{}{
		"actions": actions,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = i.request(elasticRequest{
		endpoint: "_aliases",
		method:   http.MethodPost,
		body:     bodyBytes,
	})

	return err
}

// CreateTemplate an ES index template
func (i IndicesClient) CreateTemplate(template, name string) error {
	_, err := i.request(elasticRequest{
		endpoint: fmt.Sprintf("_template/%s", name),
		method:   http.MethodPut,
		body:     []byte(template),
	})
	if err != nil {
		if responseError, isResponseError := err.(ResponseError); isResponseError {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage(fmt.Sprintf("failed to create template: %s", name))
			}
		}
		return fmt.Errorf("failed to create template: %w", err)
	}
	return nil
}

// Rollover create a rollover for certain index/alias
func (i IndicesClient) Rollover(rolloverTarget string, conditions map[string]interface{}) error {
	esReq := elasticRequest{
		endpoint: fmt.Sprintf("%s/_rollover/", rolloverTarget),
		method:   http.MethodPost,
	}
	if len(conditions) > 0 {
		body := map[string]interface{}{
			"conditions": conditions,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return err
		}
		esReq.body = bodyBytes
	}
	_, err := i.request(esReq)
	if err != nil {
		if responseError, isResponseError := err.(ResponseError); isResponseError {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage(fmt.Sprintf("failed to create rollover target: %s", rolloverTarget))
			}
		}
		return fmt.Errorf("failed to create rollover: %w", err)
	}
	return nil
}
