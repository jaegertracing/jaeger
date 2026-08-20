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

// TypedAttributeIndexingGate adds a numeric sub-field to each attribute value in
// the span index template, beside the keyword the value is already indexed as.
// That is what lets a query order on an attribute — `http.response.size > 500`
// compares lexicographically against a keyword, which makes "9" greater than
// "10" — and it is the mapping change RFC 0015 proposes. Documents are
// unaffected: a mapping does not alter _source, so nothing about reading or
// writing a span changes.
//
// Off by default because it is a spike. It costs mapped fields on the elevated
// representation, two per key instead of one, which presses hardest on a
// `tags_as_fields: all` deployment. It also only reaches indices created after
// it is turned on, so a range query against an older index matches nothing.
var TypedAttributeIndexingGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.es.typedAttributeIndexing",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.24.0"),
	featuregate.WithRegisterDescription(
		"Indexes span, resource, and event attribute values as numbers beside the "+
			"keyword, so that ordered predicates (gt/lt/gte/lte) can be answered on an "+
			"attribute. Applies only to indices created after it is enabled.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0015-typed-attribute-indexing-elasticsearch.md"),
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

// legacyIndexPattern returns the ES7 `_template` index pattern. It preserves a
// pre-M4b quirk verbatim: the span/service templates include the configured
// prefix, while dependencies/sampling omit it — both still match prefixed
// indices through the leading "*".
func (m MappingType) legacyIndexPattern(prefix string) string {
	switch m {
	case DependencyMapping, SamplingMapping:
		return "*" + m.indexBase() + "-*"
	default:
		return "*" + prefix + m.indexBase() + "-*"
	}
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
		return indices.Spans
	}
}

// innerParams are the version-independent values rendered into a neutral body.
// IsOpenSearch selects ISM vs ILM settings and is derived from the client's own
// resolved version, so it never crosses the API boundary.
type innerParams struct {
	IndexPrefix     string
	Shards          int64
	Replicas        int64
	UseILM          bool
	ILMPolicyName   string
	IsOpenSearch    bool
	TypedAttributes bool
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
	file := m.file()
	if file == "" {
		return "", fmt.Errorf("unknown index template mapping type %d", m)
	}
	opts := m.options(indices)
	if opts.Replicas == nil {
		return "", fmt.Errorf("index options for %s have no replica count configured", m)
	}
	prefix := indices.IndexPrefix.Apply("")

	var buf bytes.Buffer
	if err := indexTemplates.ExecuteTemplate(&buf, file, innerParams{
		IndexPrefix:     prefix,
		Shards:          opts.Shards,
		Replicas:        *opts.Replicas,
		UseILM:          useILM,
		ILMPolicyName:   ilmPolicyName,
		IsOpenSearch:    version.IsOpenSearch(),
		TypedAttributes: TypedAttributeIndexingGate.IsEnabled(),
	}); err != nil {
		return "", fmt.Errorf("failed to render %s index template: %w", m, err)
	}

	var inner map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &inner); err != nil {
		return "", fmt.Errorf("rendered %s index template is not valid JSON: %w", m, err)
	}

	if version.UsesV8API() {
		body, err := json.Marshal(map[string]any{
			"priority":       opts.Priority,
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
