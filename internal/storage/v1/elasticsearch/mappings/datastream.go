// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Composable-template suffixes for a data stream, following the "@" convention
// (RFC 0004 section 3.2). The data stream name is the common prefix, e.g.
// "jaeger.spans@mappings".
const (
	DataStreamMappingsSuffix = "@mappings"
	DataStreamSettingsSuffix = "@settings"
	DataStreamCustomSuffix   = "@custom"

	// dataStreamTemplatePriority is high enough to win over the default legacy
	// "jaeger-span-*" template, though the dot-notation pattern never overlaps it.
	dataStreamTemplatePriority = 500

	ismPolicyMapping = "jaeger-spans-ism-policy.json"
	ilmPolicyMapping = "jaeger-spans-ilm-policy.json"
)

// SpanDataStreamMappings renders the "@mappings" component template body for the
// spans data stream. It reuses the standard versioned span field mappings verbatim
// (so they stay in sync across versions) and adds the @timestamp field that data
// streams require for time-based partitioning. See RFC 0004 section 3.3.
func (mb *MappingBuilder) SpanDataStreamMappings() (string, error) {
	rendered, err := mb.GetMapping(SpanMapping)
	if err != nil {
		return "", err
	}
	var tmpl map[string]any
	if err := json.Unmarshal([]byte(rendered), &tmpl); err != nil {
		return "", fmt.Errorf("failed to parse span mapping: %w", err)
	}
	template, ok := tmpl["template"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("span mapping is missing the 'template' object")
	}
	spanMappings, ok := template["mappings"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("span mapping is missing the 'template.mappings' object")
	}
	properties, ok := spanMappings["properties"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("span mapping is missing the 'template.mappings.properties' object")
	}
	properties["@timestamp"] = map[string]any{"type": "date_nanos"}
	return marshalComponent(map[string]any{"mappings": spanMappings})
}

// SpanDataStreamSettings renders the "@settings" component template body: shard
// and replica counts, plus (on Elasticsearch) the ILM policy reference. On
// OpenSearch the ISM policy attaches itself by index pattern via its ism_template,
// so no settings reference is needed. See RFC 0004 sections 3.2 and 3.8.
func (mb *MappingBuilder) SpanDataStreamSettings(useILM bool, ilmPolicyName string) (string, error) {
	settings := map[string]any{
		"index.number_of_shards":            mb.Indices.Spans.Shards,
		"index.number_of_replicas":          *mb.Indices.Spans.Replicas,
		"index.mapping.nested_fields.limit": 50,
		"index.requests.cache.enable":       true,
	}
	if useILM {
		settings["index.lifecycle.name"] = ilmPolicyName
	}
	return marshalComponent(map[string]any{"settings": settings})
}

// SpanDataStreamIndexTemplate renders the composable index template that turns
// writes to dataStreamName into a data stream. It composes the @mappings,
// @settings, and (optional, user-owned) @custom component templates. See RFC 0004
// section 3.2.
func SpanDataStreamIndexTemplate(dataStreamName string) (string, error) {
	body := map[string]any{
		"index_patterns": []string{dataStreamName},
		"data_stream":    map[string]any{},
		"composed_of": []string{
			dataStreamName + DataStreamMappingsSuffix,
			dataStreamName + DataStreamSettingsSuffix,
			dataStreamName + DataStreamCustomSuffix,
		},
		"priority":                           dataStreamTemplatePriority,
		"ignore_missing_component_templates": []string{dataStreamName + DataStreamCustomSuffix},
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SpanDataStreamISMPolicy renders the OpenSearch ISM policy, whose ism_template
// auto-attaches it to the data stream's backing indices by pattern. See RFC 0004
// section 3.6.
func (mb *MappingBuilder) SpanDataStreamISMPolicy(dataStreamName string) (string, error) {
	tmpl, err := mb.TemplateBuilder.Parse(loadMapping(ismPolicyMapping))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ DataStream string }{DataStream: dataStreamName}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// SpanDataStreamILMPolicy returns the Elasticsearch ILM policy body. It is
// referenced from the @settings component template via index.lifecycle.name.
func SpanDataStreamILMPolicy() string {
	return loadMapping(ilmPolicyMapping)
}

func marshalComponent(template map[string]any) (string, error) {
	b, err := json.Marshal(map[string]any{"template": template})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
