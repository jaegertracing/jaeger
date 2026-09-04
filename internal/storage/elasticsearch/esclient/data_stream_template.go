// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// Jaeger stores spans in a data stream named for the dot-notation base below, and
// owns two component templates plus the composable index template that composes them
// (RFC 0004 §3.2). A third component, "@custom", belongs to the user: Jaeger creates
// it empty if the cluster has none and never writes to it again.
const (
	spanDataStreamBase      = "jaeger.spans"
	componentMappingsSuffix = "@mappings"
	componentSettingsSuffix = "@settings"
	componentCustomSuffix   = "@custom"

	// componentTemplateAPI and indexTemplateAPI are the composable-template
	// endpoints. A data stream is only ever declared by a composable template, so
	// these are used unconditionally, unlike CreateTemplate, whose legacy versus
	// composable choice (templateEndpoint) tracks UsesV8API.
	componentTemplateAPI = "_component_template"
	indexTemplateAPI     = "_index_template"
	dataStreamAPI        = "_data_stream"

	// dataStreamPriority is the composable index template priority from RFC 0004
	// §3.2, high enough that Jaeger's template wins over a cluster's default
	// templates. indices.spans.priority does not apply here: it tunes the rotation
	// templates on jaeger-span-*, which never compete with the exact name
	// jaeger.spans.
	dataStreamPriority = 500

	// emptyComponentBody is the body Jaeger PUTs to create a missing "@custom"
	// component. An empty settings object composes to nothing, so the component is a
	// no-op until the user puts something in it.
	emptyComponentBody = `{"template":{"settings":{}}}`
)

type dataStreamTemplate struct {
	api  string
	name string
	body string
}

// CreateSpanDataStreamTemplates installs the objects that back the span data stream
// (RFC 0004 §3.2): the "@mappings" and "@settings" component templates, the
// user-owned "@custom" component, and the composable index template that composes
// all three with "data_stream": {}. The components are created first because the
// index template references them in composed_of, which the cluster validates on
// write.
func (i IndicesClient) CreateSpanDataStreamTemplates(ctx context.Context) error {
	templates, err := renderSpanDataStreamTemplates(i.Indices)
	if err != nil {
		return err
	}
	base := i.Indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	if err := i.ensureCustomComponent(ctx, base+componentCustomSuffix); err != nil {
		return err
	}
	for _, t := range templates {
		if err := i.putComposableTemplate(ctx, t.api, t.name, "", t.body); err != nil {
			return err
		}
	}
	return nil
}

// ensureCustomComponent creates the user-owned "@custom" component template when it
// is absent, leaving an existing one untouched. Referencing it without creating it
// does not survive the supported backend matrix (RFC 0004 §3.2).
//
// The write is conditional (?create=true), so a user who creates "@custom" between
// the probe and the write keeps it: the cluster refuses Jaeger's body rather than
// replacing theirs. A refused create is settled by probing again instead of by
// reading the error message, since a template that exists now is the one to compose.
func (i IndicesClient) ensureCustomComponent(ctx context.Context, name string) error {
	exists, err := i.componentTemplateExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	createErr := i.putComposableTemplate(ctx, componentTemplateAPI, name, "create=true", emptyComponentBody)
	if createErr == nil {
		return nil
	}
	if exists, err := i.componentTemplateExists(ctx, name); err == nil && exists {
		return nil
	}
	return createErr
}

func (i IndicesClient) componentTemplateExists(ctx context.Context, name string) (bool, error) {
	_, err := i.request(ctx, elasticRequest{
		endpoint: componentTemplateAPI + "/" + name,
		method:   http.MethodGet,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if component template %q exists: %w", name, err)
	}
	return true, nil
}

// putComposableTemplate reports failures the way CreateTemplate does, so an error
// from either reads the same.
func (i IndicesClient) putComposableTemplate(ctx context.Context, api, name, query, body string) error {
	endpoint := api + "/" + name
	if query != "" {
		endpoint += "?" + query
	}
	_, err := i.request(ctx, elasticRequest{
		endpoint: endpoint,
		method:   http.MethodPut,
		body:     []byte(body),
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) {
			return responseError.prefixMessage(fmt.Sprintf("failed to create data stream template %q", name))
		}
		return fmt.Errorf("failed to create data stream template %q: %w", name, err)
	}
	return nil
}

// DeleteSpanDataStream deletes the span data stream and the backing indices it
// owns, tolerating a stream that does not exist.
//
// Purge cannot reach it through DeleteAllIndices: a data stream's backing indices
// are hidden, so the "*" wildcard leaves them in place and every span written
// before the purge is still searchable through the stream afterwards. Verified on
// Elasticsearch 7.17 and 9.4 and OpenSearch 1.3 and 3.7, which all acknowledge the
// wildcard delete and all keep the stream.
func (i IndicesClient) DeleteSpanDataStream(ctx context.Context) error {
	name := i.Indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	return i.deleteIfPresent(ctx, dataStreamAPI, name)
}

// TestsOnlyDeleteSpanDataStreamObjects removes every template
// CreateSpanDataStreamTemplates installs, tolerating any that are already absent.
// Integration-test-only: composable templates are not indices, so a suite that tears
// down with DeleteAllIndices leaves them behind.
func (i IndicesClient) TestsOnlyDeleteSpanDataStreamObjects(ctx context.Context) error {
	base := i.Indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	// The index template first: a component cannot be deleted while a template
	// composing it still exists.
	targets := []struct{ api, name string }{
		{indexTemplateAPI, base},
		{componentTemplateAPI, base + componentMappingsSuffix},
		{componentTemplateAPI, base + componentSettingsSuffix},
		{componentTemplateAPI, base + componentCustomSuffix},
	}
	for _, target := range targets {
		if err := i.deleteIfPresent(ctx, target.api, target.name); err != nil {
			return err
		}
	}
	return nil
}

func (i IndicesClient) deleteIfPresent(ctx context.Context, api, name string) error {
	_, err := i.request(ctx, elasticRequest{
		endpoint: api + "/" + name,
		method:   http.MethodDelete,
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("failed to delete %s/%s: %w", api, name, err)
	}
	return nil
}

// renderSpanDataStreamTemplates renders the three objects Jaeger owns for the span
// data stream. It asks for no lifecycle settings, because the ones the rotation path
// renders name jaeger-span-write as their rollover alias, which a data stream must
// not inherit; the stream's own policy is attached separately.
func renderSpanDataStreamTemplates(indices config.Indices) ([]dataStreamTemplate, error) {
	inner, err := renderBackendNeutralBody(SpanMapping, indices, lifecycleParams{})
	if err != nil {
		return nil, err
	}
	base := indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	return spanDataStreamTemplates(base, inner)
}

// spanDataStreamTemplates returns the objects in the order a cluster will accept
// them: both components before the index template that composes them.
func spanDataStreamTemplates(base string, inner map[string]json.RawMessage) ([]dataStreamTemplate, error) {
	// A json.RawMessage that was never set marshals as null, so a template missing
	// either field would be PUT as "mappings": null rather than named here.
	if _, ok := inner["mappings"]; !ok {
		return nil, errors.New("span index template has no mappings object")
	}
	if _, ok := inner["settings"]; !ok {
		return nil, errors.New("span index template has no settings object")
	}
	mappings, err := renderMappingsComponent(inner["mappings"])
	if err != nil {
		return nil, err
	}
	// The span index template's settings are carried through verbatim. Deriving them
	// rather than restating them is what keeps the shard and replica counts and the
	// fixed index settings from diverging between the rotation and data-stream paths.
	settings, err := marshalTemplateBody("span data stream settings",
		map[string]any{"template": map[string]any{"settings": inner["settings"]}})
	if err != nil {
		return nil, err
	}
	return []dataStreamTemplate{
		{componentTemplateAPI, base + componentMappingsSuffix, mappings},
		{componentTemplateAPI, base + componentSettingsSuffix, settings},
		{indexTemplateAPI, base, renderSpanDataStreamIndexTemplate(base)},
	}, nil
}

// renderMappingsComponent renders the "@mappings" component body: the span field
// mappings plus the "@timestamp" field every data stream must map. It decodes the
// raw mappings into its own map, so adding "@timestamp" cannot reach the caller's
// copy of the rendered body.
//
// "@timestamp" is mapped date_nanos because span start times carry microsecond
// resolution and a plain date stores milliseconds (RFC 0004 §3.3).
func renderMappingsComponent(raw json.RawMessage) (string, error) {
	var mappings map[string]any
	if err := json.Unmarshal(raw, &mappings); err != nil {
		return "", fmt.Errorf("failed to parse span index template mappings: %w", err)
	}
	properties, ok := mappings["properties"].(map[string]any)
	if !ok {
		return "", errors.New("span index template mappings have no properties object")
	}
	properties["@timestamp"] = map[string]any{"type": "date_nanos"}
	return marshalTemplateBody("span data stream mappings",
		map[string]any{"template": map[string]any{"mappings": mappings}})
}

// renderSpanDataStreamIndexTemplate renders the composable index template that
// declares the span data stream (RFC 0004 §3.2), composing Jaeger's "@mappings" and
// "@settings" components with the user-owned "@custom" one.
func renderSpanDataStreamIndexTemplate(base string) string {
	type composableTemplate struct {
		IndexPatterns []string `json:"index_patterns"`
		DataStream    struct{} `json:"data_stream"`
		ComposedOf    []string `json:"composed_of"`
		Priority      int64    `json:"priority"`
	}
	// Every field is a string or an int, so this cannot fail to serialize.
	body, _ := json.Marshal(composableTemplate{
		IndexPatterns: []string{base},
		ComposedOf: []string{
			base + componentMappingsSuffix,
			base + componentSettingsSuffix,
			base + componentCustomSuffix,
		},
		Priority: dataStreamPriority,
	})
	return string(body)
}

// marshalTemplateBody serializes a template body that embeds raw JSON from the
// rendered index template, naming what failed rather than PUTting json.Marshal's
// empty result.
func marshalTemplateBody(what string, body any) (string, error) {
	out, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal %s: %w", what, err)
	}
	return string(out), nil
}
