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

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
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
	// Index-template rendering inputs (M4b): the client renders the template
	// bodies itself, so the index config and lifecycle settings travel with the
	// client instead of a per-call render callback.
	Indices       config.Indices
	UseILM        bool
	ILMPolicyName string
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

// DeleteAllIndices deletes every index (the "*" pattern) in one request. It is
// used to wipe data, e.g. integration-test cleanup, and sends a clean DELETE /*
// rather than the comma-joined list DeleteIndices builds.
func (i *IndicesClient) DeleteAllIndices(ctx context.Context) error {
	return i.indexDeleteRequest(ctx, "*")
}

// execute delete request
func (i *IndicesClient) indexDeleteRequest(ctx context.Context, concatIndices string) error {
	// A zero master timeout is omitted so the cluster applies its own default
	// rather than master_timeout=0s, which asks the master to respond within no
	// time and can fail on a transient master delay.
	params := fmt.Sprintf("ignore_unavailable=%t", i.IgnoreUnavailableIndex)
	if i.MasterTimeoutSeconds > 0 {
		params = fmt.Sprintf("master_timeout=%ds&%s", i.MasterTimeoutSeconds, params)
	}
	_, err := i.request(ctx, elasticRequest{
		endpoint: concatIndices + "?" + params,
		method:   http.MethodDelete,
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

// CreateTemplate installs an index template for the given mapping type. The
// client renders the body itself — selecting the mapping schema and wrapping it
// in the version-appropriate envelope from its own resolved backend version — so
// callers express pure Jaeger intent and never hold a BackendVersion.
func (i IndicesClient) CreateTemplate(ctx context.Context, name string, mappingType MappingType) error {
	template, err := RenderIndexTemplate(mappingType, i.Indices, i.UseILM, i.ILMPolicyName, i.version)
	if err != nil {
		return err
	}
	_, err = i.request(ctx, elasticRequest{
		endpoint: i.templateEndpoint(name),
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

// templateEndpoint returns the index-template API path for name: the composable
// (_index_template) endpoint on backends that use the v8 API, the legacy
// (_template) endpoint otherwise. CreateTemplate and the TestsOnly template
// helpers all route through here so the endpoint choice lives in one place.
func (i IndicesClient) templateEndpoint(name string) string {
	if i.version.UsesV8API() {
		return "_index_template/" + name
	}
	return "_template/" + name
}

// TestsOnlyTemplateExists reports whether the index template for name exists,
// using the same endpoint CreateTemplate installs it under. Integration-test-only
// — production never checks a template's existence.
func (i IndicesClient) TestsOnlyTemplateExists(ctx context.Context, name string) (bool, error) {
	_, err := i.request(ctx, elasticRequest{
		endpoint: i.templateEndpoint(name),
		method:   http.MethodHead,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if template exists: %w", err)
	}
	return true, nil
}

// TestsOnlyDeleteTemplate deletes the index template for name (the same endpoint
// CreateTemplate installs it under), tolerating a missing template. Integration-
// test-only — production never deletes templates.
func (i IndicesClient) TestsOnlyDeleteTemplate(ctx context.Context, name string) error {
	_, err := i.request(ctx, elasticRequest{
		endpoint: i.templateEndpoint(name),
		method:   http.MethodDelete,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("failed to delete template %q: %w", name, err)
	}
	return nil
}

// TestsOnlyGetSettings returns the flattened settings of each named index, keyed
// by index name. Integration-test-only — production reads index settings only
// through GetJaegerIndices.
func (i IndicesClient) TestsOnlyGetSettings(ctx context.Context, indices []string) (map[string]map[string]any, error) {
	body, err := i.request(ctx, elasticRequest{
		endpoint: strings.Join(indices, ",") + "/_settings?flat_settings=true",
		method:   http.MethodGet,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get index settings: %w", err)
	}
	var raw map[string]struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal index settings: %w", err)
	}
	out := make(map[string]map[string]any, len(raw))
	for name, entry := range raw {
		out[name] = entry.Settings
	}
	return out, nil
}
