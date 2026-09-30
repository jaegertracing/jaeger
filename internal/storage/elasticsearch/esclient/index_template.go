// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"text/template"

	"go.opentelemetry.io/collector/featuregate"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// PrefixedLegacyTemplatesGate scopes the ES7/OpenSearch legacy `_template`
// index pattern of the dependencies and sampling templates by the configured
// index prefix, as the span and service templates and the composable ES8+
// template already are. Without the prefix, every prefix's dependencies and
// sampling indices match every prefix's template, and a multi-prefix cluster
// has the templates overwrite each other's settings.
//
// The gate is enabled by default; disabling it restores the unprefixed pattern.
var PrefixedLegacyTemplatesGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.es.index.prefixedLegacyTemplates",
	featuregate.StageBeta,
	featuregate.WithRegisterFromVersion("v2.22.0"),
	featuregate.WithRegisterDescription(
		"When enabled (the default), the Elasticsearch/OpenSearch legacy `_template` "+
			"index pattern for the dependencies and sampling templates includes the "+
			"configured index prefix, matching the span and service templates. Disable "+
			"it to restore the old unprefixed pattern.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/9683"),
)

//go:embed index_templates/*.json
var indexTemplateFS embed.FS

// indexTemplates holds the neutral index-template bodies, parsed once. Each
// renders the version-independent inner object (settings + mappings + optional
// aliases); renderIndexTemplate wraps it in the per-version envelope, so the
// per-version `_template`/`_index_template` split lives here rather than in the
// caller.
var indexTemplates = template.Must(template.ParseFS(indexTemplateFS, "index_templates/*.json"))

// MappingType is the Jaeger-level intent selecting which index template to
// install. The client renders and versions it internally, so callers never hold
// a BackendVersion.
type MappingType int

const (
	SpanMapping MappingType = iota
	ServiceMapping
	DependencyMapping
	SamplingMapping
)

// MappingTypeFromString resolves a Jaeger index base name (e.g. "jaeger-span")
// to its MappingType.
func MappingTypeFromString(name string) (MappingType, error) {
	switch name {
	case config.SpanIndexName:
		return SpanMapping, nil
	case config.ServiceIndexName:
		return ServiceMapping, nil
	case config.DependencyIndexName:
		return DependencyMapping, nil
	case config.SamplingIndexName:
		return SamplingMapping, nil
	default:
		return 0, fmt.Errorf("invalid mapping type: %s", name)
	}
}

// file returns the embedded neutral-body file name, or "" for an unknown type.
func (m MappingType) file() string {
	switch m {
	case SpanMapping:
		return "jaeger-span.json"
	case ServiceMapping:
		return "jaeger-service.json"
	case DependencyMapping:
		return "jaeger-dependencies.json"
	case SamplingMapping:
		return "jaeger-sampling.json"
	default:
		return ""
	}
}

// indexBase returns the dash-notation index base name for the mapping type.
func (m MappingType) indexBase() string {
	switch m {
	case ServiceMapping:
		return config.ServiceIndexName
	case DependencyMapping:
		return config.DependencyIndexName
	case SamplingMapping:
		return config.SamplingIndexName
	default:
		return config.SpanIndexName
	}
}

func (m MappingType) String() string {
	return m.indexBase()
}

// legacyIndexPattern returns the ES7 `_template` index pattern, scoped to the
// configured prefix like the template's own name and aliases. With
// PrefixedLegacyTemplatesGate disabled, the dependencies and sampling patterns
// omit the prefix.
func (m MappingType) legacyIndexPattern(prefix string) string {
	if !PrefixedLegacyTemplatesGate.IsEnabled() {
		switch m {
		case DependencyMapping, SamplingMapping:
			return "*" + m.indexBase() + "-*"
		}
	}
	return "*" + prefix + m.indexBase() + "-*"
}

// options returns the per-type index options (shards/replicas/priority).
func (m MappingType) options(indices config.Indices) config.IndexOptions {
	switch m {
	case ServiceMapping:
		return indices.Services
	case DependencyMapping:
		return indices.Dependencies
	case SamplingMapping:
		return indices.Sampling
	default:
		return indices.Spans.IndexOptions
	}
}

// spanParams are the values only the span template interpolates. They stay zero for
// every other mapping type, whose templates never read them.
type spanParams struct {
	// TotalFieldsLimit is left nil when unconfigured, so the template omits
	// "index.mapping.total_fields.limit" entirely rather than rendering a
	// default.
	TotalFieldsLimit *int64
	// NumericAttributes adds a `number` sub-field beside the keyword each attribute value is
	// indexed as, in both the nested and the elevated representation (RFC 0015 Option A). It is
	// indices.spans.numeric_attributes, which the span template alone reads. The
	// sub-field is mapped with coerce: false, so it holds only values that arrived as JSON numbers
	// and a numeric string stays out, and with ignore_malformed: true, so a value that does not fit
	// is skipped rather than costing the document. There is no boolean sub-field: OpenSearch rejects
	// ignore_malformed on a boolean mapper, and the keyword already answers equality, which is the
	// only operator a boolean has (RFC 0015 §7, question 7).
	NumericAttributes bool
}

// lifecycleParams decide whether a template hands its indices to a rollover
// lifecycle policy, and which engine runs it: Elasticsearch ILM, or the OpenSearch
// index_state_management plugin. UseILM==false leaves the other fields unread, which
// is how a target that manages its own rollover asks for no lifecycle settings.
type lifecycleParams struct {
	UseILM        bool
	ILMPolicyName string
	IsOpenSearch  bool
}

// innerParams are the values the templates in index_templates/ interpolate.
type innerParams struct {
	lifecycleParams
	IndexPrefix string
	Shards      int64
	Replicas    int64
	// Span is filled only for the span index; the other templates leave it zero and do not read it.
	Span spanParams
}

// renderBackendNeutralBody executes the embedded template for one mapping type and
// returns its top-level fields: settings, mappings, and aliases where the template
// emits them. Those fields read the same on every backend version, so a caller wraps
// them in whatever envelope its own target needs.
func renderBackendNeutralBody(m MappingType, indices config.Indices, lifecycle lifecycleParams) (map[string]json.RawMessage, error) {
	file := m.file()
	if file == "" {
		return nil, fmt.Errorf("unknown index template mapping type %d", m)
	}
	opts := m.options(indices)
	if opts.Replicas == nil {
		return nil, fmt.Errorf("index options for %s have no replica count configured", m)
	}

	params := innerParams{
		lifecycleParams: lifecycle,
		IndexPrefix:     indices.IndexPrefix.Apply(""),
		Shards:          opts.Shards,
		Replicas:        *opts.Replicas,
	}
	if m == SpanMapping {
		params.Span = spanParams{
			TotalFieldsLimit:  indices.Spans.TotalFieldsLimit.Get(),
			NumericAttributes: indices.Spans.NumericAttributes,
		}
	}

	var buf bytes.Buffer
	if err := indexTemplates.ExecuteTemplate(&buf, file, params); err != nil {
		return nil, fmt.Errorf("failed to render %s index template: %w", m, err)
	}

	var inner map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &inner); err != nil {
		return nil, fmt.Errorf("rendered %s index template is not valid JSON: %w", m, err)
	}
	return inner, nil
}

// RenderIndexTemplate renders the full index template body for a mapping type,
// wrapping the neutral inner object in the envelope required by the backend
// version: the legacy top-level `_template` shape (ES7/OpenSearch) or the
// composable `_index_template` wrapper with a priority (ES8+).
//
// CreateTemplate renders internally from the client's own resolved version, so
// online callers never pass a version. This entry point is exported only for the
// offline `esmapping-generator` CLI, which has no cluster to probe and renders a
// template for an explicitly-requested version.
func RenderIndexTemplate(m MappingType, indices config.Indices, useILM bool, ilmPolicyName string, version es.BackendVersion) (string, error) {
	prefix := indices.IndexPrefix.Apply("")
	inner, err := renderBackendNeutralBody(m, indices, lifecycleParams{
		UseILM:        useILM,
		ILMPolicyName: ilmPolicyName,
		IsOpenSearch:  version.IsOpenSearch(),
	})
	if err != nil {
		return "", err
	}

	if version.UsesV8API() {
		body, err := json.Marshal(map[string]any{
			"priority":       m.options(indices).Priority,
			"index_patterns": prefix + m.indexBase() + "-*",
			"template":       inner,
		})
		return string(body), err
	}

	// Legacy `_template`: the inner fields sit at the top level, and the index
	// pattern carries a leading "*" (preserved from the pre-M4b templates).
	inner["index_patterns"], _ = json.Marshal(m.legacyIndexPattern(prefix))
	body, err := json.Marshal(inner)
	return string(body), err
}
