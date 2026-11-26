// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/featuregate"
	"go.uber.org/zap"
)

var _ = featuregate.GlobalRegistry().MustRegister(
	"jaeger.es.config.rejectLegacyRotationFlags",
	featuregate.StageStable,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterToVersion("v2.26.0"),
	featuregate.WithRegisterDescription(
		"Validation fails when a deprecated ES rotation flag (use_aliases, use_ilm, "+
			"span_read_alias, etc.) is set.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/9016"),
)

// RejectLegacyTagsAsFields is a feature gate that, when enabled, causes validation to
// reject the deprecated top-level tags_as_fields in favor of indices.spans.tags_as_fields.
// It is enabled by default (Beta): the deprecated spelling is a validation error unless
// the gate is explicitly disabled, in which case it is honored with a deprecation warning.
var RejectLegacyTagsAsFields = featuregate.GlobalRegistry().MustRegister(
	"jaeger.es.config.rejectLegacyTagsAsFields",
	featuregate.StageBeta,
	featuregate.WithRegisterFromVersion("v2.22.0"),
	featuregate.WithRegisterDescription(
		"When enabled, the deprecated top-level 'tags_as_fields' in the Elasticsearch/OpenSearch "+
			"storage configuration becomes a validation error; use 'indices.spans.tags_as_fields' instead.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/pull/9693"),
)

// hasAnyLegacyRotationFlags returns true if any deprecated rotation-related flag was explicitly set.
func (c *Configuration) hasAnyLegacyRotationFlags() bool {
	return c.UseReadWriteAliases.HasValue() ||
		c.UseILM.HasValue() ||
		c.SpanReadAlias.HasValue() ||
		c.SpanWriteAlias.HasValue() ||
		c.ServiceReadAlias.HasValue() ||
		c.ServiceWriteAlias.HasValue()
}

// ResolvedSpanRotation returns the effective rotation configuration for span indices.
func (c *Configuration) ResolvedSpanRotation() RotationConfig {
	return resolvedRotation(&c.Indices.Spans.IndexOptions)
}

// ResolvedTagsAsFields returns the effective tags-as-fields settings: the deprecated
// top-level tags_as_fields when it is set, and indices.spans.tags_as_fields otherwise.
func (c *Configuration) ResolvedTagsAsFields() TagsAsFields {
	if legacy := c.Tags.Get(); legacy != nil {
		return *legacy
	}
	return c.Indices.Spans.Tags
}

// ResolvedServiceRotation returns the effective rotation configuration for service indices.
func (c *Configuration) ResolvedServiceRotation() RotationConfig {
	return resolvedRotation(&c.Indices.Services)
}

// ResolvedDependencyRotation returns the effective rotation configuration for dependency indices.
func (c *Configuration) ResolvedDependencyRotation() RotationConfig {
	return resolvedRotation(&c.Indices.Dependencies)
}

// ResolvedSamplingRotation returns the effective rotation configuration for sampling indices.
func (c *Configuration) ResolvedSamplingRotation() RotationConfig {
	return resolvedRotation(&c.Indices.Sampling)
}

// resolvedRotation returns the rotation that the index options configure, and periodic
// rotation when they configure none.
func resolvedRotation(idxOpts *IndexOptions) RotationConfig {
	if idxOpts.Rotation.HasRotation() {
		return idxOpts.Rotation
	}
	return RotationConfig{
		Periodic: configoptional.Some(PeriodicRotation{
			DateLayout:        idxOpts.GetDateLayout(),
			RolloverFrequency: idxOpts.GetRolloverFrequency(),
		}),
	}
}

// LogDeprecationWarnings logs a warning when the deprecated top-level tags_as_fields is set,
// advising users to move it under indices.spans.
func (c *Configuration) LogDeprecationWarnings(logger *zap.Logger) {
	if !c.Tags.HasValue() {
		return
	}
	logger.Warn(
		"Deprecated Elasticsearch configuration flag",
		zap.String("flag", "tags_as_fields"),
		zap.String("migration", "use 'indices.spans.tags_as_fields' instead; the indices.spans one is ignored while this one is set"),
	)
}
