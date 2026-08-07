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

// Data-stream span template names. The span data stream is stored under the
// dot-notation base name (config.IndexPrefix.DataStreamName), and Jaeger owns two
// component templates plus the composable index template that ties them together
// (RFC 0004 §3.2). The "@custom" component is user-owned: Jaeger creates it empty
// if it is missing, and never writes to it again.
const (
	spanDataStreamBase      = "jaeger.spans"
	componentMappingsSuffix = "@mappings"
	componentSettingsSuffix = "@settings"
	componentCustomSuffix   = "@custom"

	// componentTemplateAPI and indexTemplateAPI are the composable-template
	// endpoints. Data streams require composable templates on every backend
	// (ES 7.8+/OS 2.0+), so these are used unconditionally — unlike CreateTemplate,
	// whose legacy-vs-composable choice (templateEndpoint) tracks UsesV8API.
	componentTemplateAPI = "_component_template"
	indexTemplateAPI     = "_index_template"

	// defaultDataStreamPriority is the composable index template priority from
	// RFC 0004 §3.2: high enough that the Jaeger template wins over a cluster's
	// lower-priority default templates for the jaeger.spans pattern. A configured
	// indices.spans.priority overrides it, matching RenderIndexTemplate, which
	// honors the same setting on ES8.
	defaultDataStreamPriority = 500

	// emptyComponentBody is the body Jaeger PUTs to create a missing "@custom"
	// component. An empty settings object composes to nothing, so the component is a
	// no-op until the user puts something in it; it is spelled out rather than left
	// as a bare "template": {} because every supported backend documents component
	// templates as carrying at least one of settings, mappings or aliases.
	emptyComponentBody = `{"template":{"settings":{}}}`
)

// dataStreamTemplate is one composable object to PUT: the API path it lives under,
// its name, and its rendered body.
type dataStreamTemplate struct {
	api  string
	name string
	body string
}

// CreateDataStreamTemplates installs the objects that back the span data stream
// (RFC 0004 §3.2): the "@mappings" and "@settings" component templates, the
// user-owned "@custom" component, and the composable index template that composes
// all three with "data_stream": {}. The components are created first because the
// index template references them in composed_of, which the cluster validates on
// write.
func (i IndicesClient) CreateDataStreamTemplates(ctx context.Context) error {
	templates, err := renderSpanDataStreamTemplates(i.Indices)
	if err != nil {
		return err
	}
	base := i.Indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	if err := i.ensureCustomComponent(ctx, base+componentCustomSuffix); err != nil {
		return err
	}
	for _, t := range templates {
		if err := i.putComposableTemplate(ctx, t.api, t.name, t.body); err != nil {
			return err
		}
	}
	return nil
}

// ensureCustomComponent creates the user-owned "@custom" component template when it
// does not already exist, leaving an existing one untouched.
//
// RFC 0004 §3.2 describes referencing "@custom" without creating it, which does not
// survive the backend matrix: ignore_missing_component_templates was added in ES 8.7
// and exists in no OpenSearch version, and both parsers reject unknown fields, while
// dropping the field is not sufficient either because OpenSearch rejects a
// composed_of naming a template that does not exist. Creating it empty when absent
// gives one index-template body that is valid everywhere, with the contents still
// owned by the user.
func (i IndicesClient) ensureCustomComponent(ctx context.Context, name string) error {
	exists, err := i.componentTemplateExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return i.putComposableTemplate(ctx, componentTemplateAPI, name, emptyComponentBody)
}

// componentTemplateExists reports whether a component template exists. It uses GET
// rather than HEAD because HEAD on the component-template endpoint is not uniformly
// available across the supported ES/OpenSearch matrix.
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

// putComposableTemplate PUTs a composable component or index template body. It
// mirrors CreateTemplate's response handling so failures read the same way.
func (i IndicesClient) putComposableTemplate(ctx context.Context, api, name, body string) error {
	_, err := i.request(ctx, elasticRequest{
		endpoint: api + "/" + name,
		method:   http.MethodPut,
		body:     []byte(body),
	})
	if err != nil {
		var responseError ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode != http.StatusOK {
			return responseError.prefixMessage(fmt.Sprintf("failed to create data stream template %q", name))
		}
		return fmt.Errorf("failed to create data stream template %q: %w", name, err)
	}
	return nil
}

// TestsOnlyDeleteDataStreamObjects removes the span data stream, its backing
// indices, and every template CreateDataStreamTemplates installs, tolerating any
// that are already absent. Integration-test-only: a data stream's backing indices
// are hidden, so the suite's DeleteAllIndices("*") teardown does not reclaim them,
// and the composable endpoints used here are the same on every backend version.
func (i IndicesClient) TestsOnlyDeleteDataStreamObjects(ctx context.Context) error {
	base := i.Indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	// The data stream first: a template cannot be deleted while one composed from
	// it is still in use.
	targets := []struct{ api, name string }{
		{"_data_stream", base},
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

// deleteIfPresent DELETEs a composable object, treating a missing one as success.
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

// renderSpanDataStreamTemplates renders the span data-stream objects Jaeger owns.
//
// The neutral body is rendered with lifecycle off: the read alias and the ILM/ISM
// settings are the only fields that consume IndexPrefix, and both carry rotation-path
// names a data stream must not inherit. The data stream's own lifecycle policy is
// attached separately.
func renderSpanDataStreamTemplates(indices config.Indices) ([]dataStreamTemplate, error) {
	inner, err := renderNeutralBody(SpanMapping, indices, innerParams{})
	if err != nil {
		return nil, err
	}
	base := indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	return spanDataStreamTemplates(base, indices.Spans.Priority, inner)
}

// spanDataStreamTemplates builds the objects in the order they must be created. It
// takes an already-rendered body rather than the config so its failure paths — a
// body whose mappings or settings cannot become a component — are directly
// unit-testable rather than unreachable behind the embedded template.
func spanDataStreamTemplates(base string, priority int64, inner map[string]json.RawMessage) ([]dataStreamTemplate, error) {
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
	indexTemplate, err := renderSpanDataStreamIndexTemplate(base, priority)
	if err != nil {
		return nil, err
	}
	return []dataStreamTemplate{
		{componentTemplateAPI, base + componentMappingsSuffix, mappings},
		{componentTemplateAPI, base + componentSettingsSuffix, settings},
		{indexTemplateAPI, base, indexTemplate},
	}, nil
}

// renderMappingsComponent renders the "@mappings" component body: the span field
// mappings plus the "@timestamp" field every data stream must map. It decodes the
// raw mappings into its own map, so adding "@timestamp" cannot reach the caller's
// copy of the rendered body.
//
// "@timestamp" is date_nanos to preserve the nanosecond precision Jaeger writes.
// The write path emits an RFC 3339 nanosecond string rather than epoch nanoseconds:
// date_nanos reads a bare number as epoch *milliseconds*, which puts epoch-nanosecond
// input past the type's 2262 ceiling and gets the document rejected.
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
func renderSpanDataStreamIndexTemplate(base string, priority int64) (string, error) {
	type composableTemplate struct {
		IndexPatterns []string `json:"index_patterns"`
		DataStream    struct{} `json:"data_stream"`
		ComposedOf    []string `json:"composed_of"`
		Priority      int64    `json:"priority"`
	}
	if priority == 0 {
		priority = defaultDataStreamPriority
	}
	return marshalTemplateBody("span data stream index template", composableTemplate{
		IndexPatterns: []string{base},
		ComposedOf: []string{
			base + componentMappingsSuffix,
			base + componentSettingsSuffix,
			base + componentCustomSuffix,
		},
		Priority: priority,
	})
}

// marshalTemplateBody serializes a template body. Every PUT body is built here, so a
// payload that cannot serialize fails the call rather than leaving json.Marshal's
// empty result behind and PUTting an empty template.
func marshalTemplateBody(what string, body any) (string, error) {
	out, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal %s: %w", what, err)
	}
	return string(out), nil
}
