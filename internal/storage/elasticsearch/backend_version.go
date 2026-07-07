// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// BackendVersion identifies the Elasticsearch or OpenSearch backend version.
// It encodes both the flavor (ES vs OpenSearch) and major version number,
// replacing the separate Version uint + IsOpenSearch bool that were previously
// used independently.
type BackendVersion uint

const (
	ElasticV7   BackendVersion = 7
	ElasticV8   BackendVersion = 8
	ElasticV9   BackendVersion = 9
	OpenSearch1 BackendVersion = 101
	OpenSearch2 BackendVersion = 102
	OpenSearch3 BackendVersion = 103
)

// AllVersions lists every backend major version Jaeger supports: Elasticsearch
// 7/8/9 and OpenSearch 1/2/3. Elasticsearch 6 reached EOL and is no longer
// supported.
var AllVersions = []BackendVersion{
	ElasticV7,
	ElasticV8,
	ElasticV9,
	OpenSearch1,
	OpenSearch2,
	OpenSearch3,
}

// IsSupportedVersion reports whether v is a version number Jaeger accepts as an
// explicit config.Version override. 0 (auto-detect) is not itself a version and
// returns false; callers treat 0 specially.
func IsSupportedVersion(v uint) bool {
	return slices.Contains(AllVersions, BackendVersion(v))
}

func (v BackendVersion) String() string {
	switch v {
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

// TemplateVersion returns the ES template version to use (7 or 8).
// OpenSearch uses ES 7.x templates; ES 9+ uses ES 8.x templates.
func (v BackendVersion) TemplateVersion() uint {
	if v.IsOpenSearch() || v == ElasticV7 {
		return 7
	}
	return 8
}

// UsesV8API returns true if the backend requires the v8 index template API.
func (v BackendVersion) UsesV8API() bool {
	return v == ElasticV8 || v == ElasticV9
}

// PingResult holds the version fields Jaeger reads from an Elasticsearch or
// OpenSearch root document ("GET /"), independent of which HTTP client fetched
// it. It is the input to the shared version-resolution path.
type PingResult struct {
	// VersionNumber is the raw version string (e.g. "7.10.2").
	VersionNumber string
	// TagLine distinguishes OpenSearch from Elasticsearch.
	TagLine string
}

// ResolveBackendVersion is the single version-detection path shared by the
// data-plane and admin-plane client builders. It returns the configured version
// when it is non-zero (an explicit override, honored without a network call);
// otherwise it calls ping once and derives the version from the response.
func ResolveBackendVersion(ctx context.Context, configured uint, ping func(context.Context) (PingResult, error)) (BackendVersion, error) {
	if configured != 0 {
		return BackendVersion(configured), nil
	}
	result, err := ping(ctx)
	if err != nil {
		return 0, err
	}
	if result.VersionNumber == "" {
		return 0, errors.New("backend returned an empty version number")
	}
	// Parse the whole major component (up to the first dot), not just the first
	// byte — otherwise "10.x" would be misread as major 1.
	majorVersion, err := strconv.Atoi(strings.Split(result.VersionNumber, ".")[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version format: %s", result.VersionNumber)
	}
	return DetectBackendVersion(result.TagLine, majorVersion), nil
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
	case 7:
		return ElasticV7
	case 9:
		return ElasticV9
	default:
		return ElasticV8
	}
}
