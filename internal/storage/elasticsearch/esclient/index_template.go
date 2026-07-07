// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"text/template"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
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
	IndexPrefix   string
	Shards        int64
	Replicas      int64
	UseILM        bool
	ILMPolicyName string
	IsOpenSearch  bool
}

// renderIndexTemplate renders the full index template body for a mapping type,
// wrapping the neutral inner object in the envelope required by the backend
// version: the legacy top-level `_template` shape (ES7/OpenSearch) or the
// composable `_index_template` wrapper with a priority (ES8+).
func renderIndexTemplate(m MappingType, indices config.Indices, useILM bool, ilmPolicyName string, version es.BackendVersion) (string, error) {
	file := m.file()
	if file == "" {
		return "", fmt.Errorf("unknown index template mapping type %d", m)
	}
	opts := m.options(indices)
	prefix := indices.IndexPrefix.Apply("")

	var buf bytes.Buffer
	if err := indexTemplates.ExecuteTemplate(&buf, file, innerParams{
		IndexPrefix:   prefix,
		Shards:        opts.Shards,
		Replicas:      *opts.Replicas,
		UseILM:        useILM,
		ILMPolicyName: ilmPolicyName,
		IsOpenSearch:  version.IsOpenSearch(),
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
