// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
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
// (RFC 0004 §3.2). The user-controlled "@custom" component is referenced but never
// created by Jaeger.
const (
	spanDataStreamBase      = "jaeger.spans"
	componentMappingsSuffix = "@mappings"
	componentSettingsSuffix = "@settings"
	componentCustomSuffix   = "@custom"

	// componentTemplateAPI and composableTemplateAPI are the composable-template
	// endpoints. Data streams require composable templates on every backend
	// (ES 7.8+/OS 2.0+), so these are used unconditionally — unlike CreateTemplate,
	// whose legacy-vs-composable choice (templateEndpoint) tracks UsesV8API.
	componentTemplateAPI  = "_component_template"
	composableTemplateAPI = "_index_template"

	// dataStreamTemplatePriority is the composable index template priority from
	// RFC 0004 §3.2: high enough that the Jaeger template wins over a cluster's
	// lower-priority default templates for the jaeger.spans pattern.
	dataStreamTemplatePriority = 500
)

// dataStreamTemplate is one composable object to PUT: the API path it lives under,
// its name, and its rendered body.
type dataStreamTemplate struct {
	api  string
	name string
	body string
}

// spanIndexTemplateBody is the decoded form of the version-neutral span index
// template (index_templates/jaeger-span.json) that the rotation path also renders.
// Settings are held as raw JSON so the data stream reuses the span index's own bytes
// rather than restating them; mappings are decoded because a data stream adds a field.
type spanIndexTemplateBody struct {
	Settings json.RawMessage `json:"settings"`
	Mappings map[string]any  `json:"mappings"`
}

// CreateDataStreamTemplates installs the three objects that back the span data
// stream (RFC 0004 §3.2): the "@mappings" and "@settings" component templates and
// the composable index template that composes them with "data_stream": {}. The
// component templates are created first because the index template references them
// in composed_of, which the cluster validates on write.
//
// It is dormant today — no factory path calls it yet (the write path landed
// dormant in #8833); the startup wiring and the config gate that turn the feature
// on follow in later slices.
func (i IndicesClient) CreateDataStreamTemplates(ctx context.Context) error {
	templates, err := renderSpanDataStreamTemplates(i.Indices)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if err := i.putComposableTemplate(ctx, t.api, t.name, t.body); err != nil {
			return err
		}
	}
	return nil
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

// renderSpanDataStreamTemplates renders the three span data-stream objects from the
// span index template.
func renderSpanDataStreamTemplates(indices config.Indices) ([]dataStreamTemplate, error) {
	body, err := renderSpanIndexTemplateBody(indices)
	if err != nil {
		return nil, err
	}
	return spanDataStreamTemplates(indices.IndexPrefix.DataStreamName(spanDataStreamBase), body)
}

// spanDataStreamTemplates builds the three objects in the order they must be
// created. It takes an already-rendered body rather than the config so its failure
// paths — a body whose mappings or settings cannot become a component — are directly
// unit-testable rather than unreachable behind the embedded template.
func spanDataStreamTemplates(base string, body spanIndexTemplateBody) ([]dataStreamTemplate, error) {
	mappings, err := renderMappingsComponent(body.Mappings)
	if err != nil {
		return nil, err
	}
	settings, err := renderSettingsComponent(body.Settings)
	if err != nil {
		return nil, err
	}
	indexTemplate, err := renderSpanDataStreamIndexTemplate(base)
	if err != nil {
		return nil, err
	}
	return []dataStreamTemplate{
		{componentTemplateAPI, base + componentMappingsSuffix, mappings},
		{componentTemplateAPI, base + componentSettingsSuffix, settings},
		{composableTemplateAPI, base, indexTemplate},
	}, nil
}

// renderSpanIndexTemplateBody renders index_templates/jaeger-span.json with
// lifecycle off. The read alias and the ILM/ISM settings are the only fields that
// consume IndexPrefix, and both carry rotation-path names a data stream must not
// inherit; the data stream's own lifecycle policy is attached in a later slice.
func renderSpanIndexTemplateBody(indices config.Indices) (spanIndexTemplateBody, error) {
	opts := indices.Spans
	if opts.Replicas == nil {
		return spanIndexTemplateBody{}, errors.New("span index options have no replica count configured")
	}
	var buf bytes.Buffer
	if err := indexTemplates.ExecuteTemplate(&buf, SpanMapping.file(), innerParams{
		Shards:   opts.Shards,
		Replicas: *opts.Replicas,
	}); err != nil {
		return spanIndexTemplateBody{}, fmt.Errorf("failed to render span index template: %w", err)
	}
	return parseSpanIndexTemplateBody(buf.Bytes())
}

// parseSpanIndexTemplateBody decodes a rendered span index template. It takes bytes
// so its failure paths are directly unit-testable rather than unreachable behind the
// embedded template.
func parseSpanIndexTemplateBody(rendered []byte) (spanIndexTemplateBody, error) {
	var body spanIndexTemplateBody
	if err := json.Unmarshal(rendered, &body); err != nil {
		return spanIndexTemplateBody{}, fmt.Errorf("failed to parse span index template: %w", err)
	}
	if len(body.Settings) == 0 {
		return spanIndexTemplateBody{}, errors.New("span index template has no settings object")
	}
	return body, nil
}

// renderMappingsComponent renders the "@mappings" component body: the span field
// mappings plus the "@timestamp" field every data stream must map (date_nanos
// preserves the nanosecond precision Jaeger writes, RFC 0004 §3.3).
func renderMappingsComponent(mappings map[string]any) (string, error) {
	properties, ok := mappings["properties"].(map[string]any)
	if !ok {
		return "", errors.New("span index template mappings have no properties object")
	}
	properties["@timestamp"] = map[string]any{"type": "date_nanos"}
	return marshalTemplateBody("span data stream mappings",
		map[string]any{"template": map[string]any{"mappings": mappings}})
}

// renderSettingsComponent renders the "@settings" component body from the span index
// template's own settings, carried through as raw JSON. Deriving them rather than
// restating them is what keeps the shard and replica counts and the fixed index
// settings from diverging between the rotation and data-stream write paths.
func renderSettingsComponent(settings json.RawMessage) (string, error) {
	return marshalTemplateBody("span data stream settings",
		map[string]any{"template": map[string]any{"settings": settings}})
}

// renderSpanDataStreamIndexTemplate renders the composable index template that
// declares the span data stream (RFC 0004 §3.2). It composes Jaeger's "@mappings"
// and "@settings" components with the user-controlled "@custom" one, which is marked
// ignore_missing so the template is valid before the user creates it.
func renderSpanDataStreamIndexTemplate(base string) (string, error) {
	type composableTemplate struct {
		IndexPatterns                   []string `json:"index_patterns"`
		DataStream                      struct{} `json:"data_stream"`
		ComposedOf                      []string `json:"composed_of"`
		Priority                        int      `json:"priority"`
		IgnoreMissingComponentTemplates []string `json:"ignore_missing_component_templates"`
	}
	return marshalTemplateBody("span data stream index template", composableTemplate{
		IndexPatterns: []string{base},
		ComposedOf: []string{
			base + componentMappingsSuffix,
			base + componentSettingsSuffix,
			base + componentCustomSuffix,
		},
		Priority:                        dataStreamTemplatePriority,
		IgnoreMissingComponentTemplates: []string{base + componentCustomSuffix},
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
