// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"fmt"
	"strings"
)

// BackendVersion identifies the Elasticsearch or OpenSearch backend version.
// It encodes both the flavor (ES vs OpenSearch) and major version number,
// replacing the separate Version uint + IsOpenSearch bool that were previously
// used independently.
type BackendVersion uint

const (
	ElasticV6   BackendVersion = 6
	ElasticV7   BackendVersion = 7
	ElasticV8   BackendVersion = 8
	ElasticV9   BackendVersion = 9
	OpenSearch1 BackendVersion = 101
	OpenSearch2 BackendVersion = 102
	OpenSearch3 BackendVersion = 103
)

// AllVersions lists every backend major version Jaeger supports from a single
// binary (§6 G3 of RFC 0006): Elasticsearch 6/7/8/9 and OpenSearch 1/2/3.
var AllVersions = []BackendVersion{
	ElasticV6,
	ElasticV7,
	ElasticV8,
	ElasticV9,
	OpenSearch1,
	OpenSearch2,
	OpenSearch3,
}

func (v BackendVersion) String() string {
	switch v {
	case ElasticV6:
		return "Elasticsearch 6.x"
	case ElasticV7:
		return "Elasticsearch 7.x"
	case ElasticV8:
		return "Elasticsearch 8.x"
	case ElasticV9:
		return "Elasticsearch 9.x"
	case OpenSearch1:
		return "OpenSearch 1.x"
	case OpenSearch2:
		return "OpenSearch 2.x"
	case OpenSearch3:
		return "OpenSearch 3.x"
	default:
		return fmt.Sprintf("Unknown(%d)", v)
	}
}

// IsOpenSearch returns true if the backend is OpenSearch.
func (v BackendVersion) IsOpenSearch() bool {
	return v >= OpenSearch1
}

// TemplateVersion returns the ES template version to use (6, 7, or 8).
// OpenSearch uses ES 7.x templates; ES 9+ uses ES 8.x templates.
func (v BackendVersion) TemplateVersion() uint {
	if v.IsOpenSearch() {
		return 7
	}
	switch v {
	case ElasticV6:
		return 6
	case ElasticV7:
		return 7
	default:
		return 8
	}
}

// UsesV8API returns true if the backend requires the v8 index template API.
func (v BackendVersion) UsesV8API() bool {
	return v == ElasticV8 || v == ElasticV9
}

// SupportsTypedIndices returns true if index requests require a _type parameter.
// Only ES 6.x requires this; ES 7+ and all OpenSearch versions ignore it.
func (v BackendVersion) SupportsTypedIndices() bool {
	return v == ElasticV6
}

// SupportsILM returns true if the backend supports Index Lifecycle Management.
// ILM requires ES 7+ or OpenSearch (which uses ISM, the equivalent feature).
func (v BackendVersion) SupportsILM() bool {
	return v != ElasticV6
}

// DetectBackendVersion determines the BackendVersion from the ping response.
func DetectBackendVersion(tagLine string, majorVersion int) BackendVersion {
	if strings.Contains(tagLine, "OpenSearch") {
		switch majorVersion {
		case 1:
			return OpenSearch1
		case 2:
			return OpenSearch2
		default:
			return OpenSearch3
		}
	}
	switch majorVersion {
	case 6:
		return ElasticV6
	case 7:
		return ElasticV7
	case 9:
		return ElasticV9
	default:
		return ElasticV8
	}
}
