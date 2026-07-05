// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"errors"
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
	// IsWriteIndex option
	IsWriteIndex bool
}

var _ IndexAPI = (*IndicesClient)(nil)

// IndicesClient is a client used to manipulate indices.
type IndicesClient struct {
	Client
	MasterTimeoutSeconds   int
	IgnoreUnavailableIndex bool
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
func (i *IndicesClient) GetJaegerIndices(ctx context.Context, prefix string) ([]Index, error) {
	prefix += "jaeger-*"

	body, err := i.request(ctx, elasticRequest{
		endpoint: prefix + "?flat_settings=true&filter_path=*.aliases,*.settings",
		method:   http.MethodGet,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query indices: %w", err)
	}

	type indexInfo struct {
		Aliases  map[string]any    `json:"aliases"`
		Settings map[string]string `json:"settings"`
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
func (i *IndicesClient) indexDeleteRequest(ctx context.Context, concatIndices string) error {
	_, err := i.request(ctx, elasticRequest{
		endpoint: fmt.Sprintf("%s?master_timeout=%ds&ignore_unavailable=%t", concatIndices,
			i.MasterTimeoutSeconds, i.IgnoreUnavailableIndex),
		method: http.MethodDelete,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to delete indices: " + concatIndices)
			}
		}
		return fmt.Errorf("failed to delete indices: %w", err)
	}
	return nil
}

// DeleteIndices deletes specified set of indices.
func (i *IndicesClient) DeleteIndices(ctx context.Context, indices []Index) error {
	concatIndices := ""
	for j, index := range indices {
		// verify the length of the concatIndices
		// An HTTP line is should not be larger than 4096 bytes
		// a line contains other than concatIndices data in the request, ie: master_timeout
		// for a safer side check the line length should not exceed 4000
		if (len(concatIndices) + len(index.Index)) > 4000 {
			err := i.indexDeleteRequest(ctx, concatIndices)
			if err != nil {
				return err
			}
			concatIndices = ""
		}

		concatIndices += index.Index
		concatIndices += ","

		// if it is last index, delete request should be executed
		if j == len(indices)-1 {
			return i.indexDeleteRequest(ctx, concatIndices)
		}
	}
	return nil
}

// CreateIndex an ES index
func (i *IndicesClient) CreateIndex(ctx context.Context, index string) error {
	_, err := i.request(ctx, elasticRequest{
		endpoint: index,
		method:   http.MethodPut,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to create index: " + index)
			}
		}
		return fmt.Errorf("failed to create index: %w", err)
	}
	return nil
}

// CreateAlias an ES specific set of index aliases
func (i *IndicesClient) CreateAlias(ctx context.Context, aliases []Alias) error {
	err := i.aliasAction(ctx, "add", aliases)
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to create aliases: " + i.aliasesString(aliases))
			}
		}
		return fmt.Errorf("failed to create aliases: %w", err)
	}
	return nil
}

// DeleteAlias an ES specific set of index aliases
func (i *IndicesClient) DeleteAlias(ctx context.Context, aliases []Alias) error {
	err := i.aliasAction(ctx, "remove", aliases)
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to delete aliases: " + i.aliasesString(aliases))
			}
		}
		return fmt.Errorf("failed to delete aliases: %w", err)
	}
	return nil
}

// AliasExists check whether an alias exists or not
func (i *IndicesClient) AliasExists(ctx context.Context, alias string) (bool, error) {
	_, err := i.request(ctx, elasticRequest{
		endpoint: "_alias/" + alias,
		method:   http.MethodHead,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode == http.StatusNotFound {
				return false, nil
			}
		}
		return false, fmt.Errorf("failed to check if alias exists: %w", err)
	}
	return true, nil
}

// IndexExists check whether an index exists or not
func (i *IndicesClient) IndexExists(ctx context.Context, index string) (bool, error) {
	_, err := i.request(ctx, elasticRequest{
		endpoint: index,
		method:   http.MethodHead,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode == http.StatusNotFound {
				return false, nil
			}
		}
		return false, fmt.Errorf("failed to check if index exists: %w", err)
	}
	return true, nil
}

func (*IndicesClient) aliasesString(aliases []Alias) string {
	var builder strings.Builder
	for _, alias := range aliases {
		fmt.Fprintf(&builder, "[index: %s, alias: %s],", alias.Index, alias.Name)
	}
	concatAliases := builder.String()
	return strings.Trim(concatAliases, ",")
}

func (i *IndicesClient) aliasAction(ctx context.Context, action string, aliases []Alias) error {
	actions := []map[string]any{}

	for _, alias := range aliases {
		options := map[string]any{
			"index": alias.Index,
			"alias": alias.Name,
		}
		if alias.IsWriteIndex {
			options["is_write_index"] = true
		}
		actions = append(actions, map[string]any{
			action: options,
		})
	}

	body := map[string]any{
		"actions": actions,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = i.request(ctx, elasticRequest{
		endpoint: "_aliases",
		method:   http.MethodPost,
		body:     bodyBytes,
	})

	return err
}

// CreateTemplate an ES index template
func (i IndicesClient) CreateTemplate(ctx context.Context, template, name string) error {
	if i.Version == 0 {
		return errors.New("client version is unset")
	}

	endpointFmt := "_template/%s"
	if i.Version.UsesV8API() {
		endpointFmt = "_index_template/%s"
	}
	_, err := i.request(ctx, elasticRequest{
		endpoint: fmt.Sprintf(endpointFmt, name),
		method:   http.MethodPut,
		body:     []byte(template),
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to create template: " + name)
			}
		}
		return fmt.Errorf("failed to create template: %w", err)
	}
	return nil
}

// Rollover create a rollover for certain index/alias
func (i IndicesClient) Rollover(ctx context.Context, rolloverTarget string, conditions map[string]any) error {
	esReq := elasticRequest{
		endpoint: rolloverTarget + "/_rollover/",
		method:   http.MethodPost,
	}
	if len(conditions) > 0 {
		body := map[string]any{
			"conditions": conditions,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return err
		}
		esReq.body = bodyBytes
	}
	_, err := i.request(ctx, esReq)
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			if responseError.StatusCode != http.StatusOK {
				return responseError.prefixMessage("failed to create rollover target: " + rolloverTarget)
			}
		}
		return fmt.Errorf("failed to create rollover: %w", err)
	}
	return nil
}
