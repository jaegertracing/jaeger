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

// renderSpanDataStreamTemplates renders the three span data-stream objects in the
// order they must be created: the two component templates first, then the index
// template that composes them.
func renderSpanDataStreamTemplates(indices config.Indices) ([]dataStreamTemplate, error) {
	base := indices.IndexPrefix.DataStreamName(spanDataStreamBase)
	mappings, err := renderSpanDataStreamMappings()
	if err != nil {
		return nil, err
	}
	settings, err := renderSpanDataStreamSettings(indices)
	if err != nil {
		return nil, err
	}
	return []dataStreamTemplate{
		{componentTemplateAPI, base + componentMappingsSuffix, mappings},
		{componentTemplateAPI, base + componentSettingsSuffix, settings},
		{composableTemplateAPI, base, renderSpanDataStreamIndexTemplate(base)},
	}, nil
}

// renderSpanDataStreamMappings renders the "@mappings" component template body:
// the span field mappings plus the data-stream "@timestamp" field. The field
// mappings are lifted from the span index template (jaeger-span.json) so the data
// stream stays a single source of truth with the rotation path instead of
// duplicating the schema; a drift test guards that they match.
func renderSpanDataStreamMappings() (string, error) {
	var buf bytes.Buffer
	if err := indexTemplates.ExecuteTemplate(&buf, SpanMapping.file(), innerParams{}); err != nil {
		return "", fmt.Errorf("failed to render span mappings: %w", err)
	}
	return spanMappingsComponent(buf.Bytes())
}

// spanMappingsComponent extracts the "mappings" object from a rendered span index
// template. It takes the rendered template as bytes so its parse failure is
// directly unit-testable rather than unreachable.
func spanMappingsComponent(renderedSpanTemplate []byte) (string, error) {
	var parsed struct {
		Mappings map[string]any `json:"mappings"`
	}
	if err := json.Unmarshal(renderedSpanTemplate, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse span mappings: %w", err)
	}
	return mappingsComponentBody(parsed.Mappings)
}

// mappingsComponentBody wraps a mappings object as a component-template body,
// adding the "@timestamp" field data streams require (date_nanos preserves the
// OTLP nanosecond precision Jaeger writes, see RFC 0004 §3.3).
func mappingsComponentBody(mappings map[string]any) (string, error) {
	properties, ok := mappings["properties"].(map[string]any)
	if !ok {
		return "", errors.New("span index template mappings have no properties object")
	}
	properties["@timestamp"] = map[string]any{"type": "date_nanos"}

	out, err := json.Marshal(map[string]any{"template": map[string]any{"mappings": mappings}})
	if err != nil {
		return "", fmt.Errorf("failed to marshal span data stream mappings: %w", err)
	}
	return string(out), nil
}

// renderSpanDataStreamSettings renders the "@settings" component template body:
// the span shard/replica counts and the fixed index settings, matching the span
// index template. The lifecycle-policy reference is intentionally left out — it is
// added with the default ISM/ILM policy in a later slice.
func renderSpanDataStreamSettings(indices config.Indices) (string, error) {
	opts := indices.Spans
	if opts.Replicas == nil {
		return "", errors.New("span index options have no replica count configured")
	}
	out, _ := json.Marshal(map[string]any{
		"template": map[string]any{
			"settings": map[string]any{
				"index.number_of_shards":            opts.Shards,
				"index.number_of_replicas":          *opts.Replicas,
				"index.mapping.nested_fields.limit": 50,
				"index.requests.cache.enable":       true,
			},
		},
	})
	return string(out), nil
}

// renderSpanDataStreamIndexTemplate renders the composable index template that
// declares the span data stream (RFC 0004 §3.2). It composes Jaeger's "@mappings"
// and "@settings" components with the user-controlled "@custom" one, which is
// marked ignore_missing so the template is valid before the user creates it.
func renderSpanDataStreamIndexTemplate(base string) string {
	// This body is fixed-shape (only strings and ints), so marshaling cannot fail.
	type composableTemplate struct {
		IndexPatterns                   []string `json:"index_patterns"`
		DataStream                      struct{} `json:"data_stream"`
		ComposedOf                      []string `json:"composed_of"`
		Priority                        int      `json:"priority"`
		IgnoreMissingComponentTemplates []string `json:"ignore_missing_component_templates"`
	}
	out, _ := json.Marshal(composableTemplate{
		IndexPatterns: []string{base},
		ComposedOf: []string{
			base + componentMappingsSuffix,
			base + componentSettingsSuffix,
			base + componentCustomSuffix,
		},
		Priority:                        dataStreamTemplatePriority,
		IgnoreMissingComponentTemplates: []string{base + componentCustomSuffix},
	})
	return string(out)
}
