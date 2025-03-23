// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "strings"

// DotReplacer replaces the dots in span tags
type DotReplacer struct {
	tagDotReplacement string
}

// NewDotReplacer returns an instance of DotReplacer
func NewDotReplacer(tagDotReplacement string) DotReplacer {
	return DotReplacer{tagDotReplacement: tagDotReplacement}
}

// ReplaceDot replaces dot with dotReplacement
func (dm DotReplacer) ReplaceDot(k string) string {
	return strings.ReplaceAll(k, ".", dm.tagDotReplacement)
}

// ReplaceDotReplacement replaces dotReplacement with dot
func (dm DotReplacer) ReplaceDotReplacement(k string) string {
	return strings.ReplaceAll(k, dm.tagDotReplacement, ".")
}
