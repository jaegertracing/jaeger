// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/featuregate"
	"go.uber.org/zap"
)

// RejectLegacyRotationFlags is a feature gate that, when enabled, causes validation
// to reject deprecated rotation-related flags (use_aliases, use_ilm,
// span_read_alias, span_write_alias, service_read_alias, service_write_alias).
// Once promoted to Stable, users must migrate to the new rotation config.
var RejectLegacyRotationFlags = featuregate.GlobalRegistry().MustRegister(
	"es.config.rejectLegacyRotationFlags",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.9.0"),
	featuregate.WithRegisterDescription(
		"When enabled, the use of deprecated ES rotation flags "+
			"(use_aliases, use_ilm, span_read_alias, etc.) "+
			"becomes a validation error instead of a deprecation warning.",
	),
)

func (c *Configuration) getUseReadWriteAliases() bool {
	if p := c.UseReadWriteAliases.Get(); p != nil {
		return *p
	}
	return false
}

func (c *Configuration) getUseILM() bool {
	if p := c.UseILM.Get(); p != nil {
		return *p
	}
	return false
}

func (c *Configuration) getSpanReadAlias() string {
	if p := c.SpanReadAlias.Get(); p != nil {
		return *p
	}
	return ""
}

func (c *Configuration) getSpanWriteAlias() string {
	if p := c.SpanWriteAlias.Get(); p != nil {
		return *p
	}
	return ""
}

func (c *Configuration) getServiceReadAlias() string {
	if p := c.ServiceReadAlias.Get(); p != nil {
		return *p
	}
	return ""
}

func (c *Configuration) getServiceWriteAlias() string {
	if p := c.ServiceWriteAlias.Get(); p != nil {
		return *p
	}
	return ""
}

// hasAnyLegacyRotationFlags returns true if any deprecated rotation-related flag was explicitly set.
func (c *Configuration) hasAnyLegacyRotationFlags() bool {
	return c.UseReadWriteAliases.HasValue() ||
		c.UseILM.HasValue() ||
		c.SpanReadAlias.HasValue() ||
		c.SpanWriteAlias.HasValue() ||
		c.ServiceReadAlias.HasValue() ||
		c.ServiceWriteAlias.HasValue()
}

// ResolvedSpanRotation returns the effective rotation configuration for span indices,
// resolving legacy flags into the appropriate RotationConfig variant.
func (c *Configuration) ResolvedSpanRotation(prefix string) RotationConfig {
	return c.resolvedRotation(&c.Indices.Spans, prefix, c.getSpanReadAlias(), c.getSpanWriteAlias())
}

// ResolvedServiceRotation returns the effective rotation configuration for service indices,
// resolving legacy flags into the appropriate RotationConfig variant.
func (c *Configuration) ResolvedServiceRotation(prefix string) RotationConfig {
	return c.resolvedRotation(&c.Indices.Services, prefix, c.getServiceReadAlias(), c.getServiceWriteAlias())
}

// ResolvedDependencyRotation returns the effective rotation configuration for dependency indices.
func (c *Configuration) ResolvedDependencyRotation(prefix string) RotationConfig {
	return c.resolvedRotation(&c.Indices.Dependencies, prefix, "", "")
}

// ResolvedSamplingRotation returns the effective rotation configuration for sampling indices.
func (c *Configuration) ResolvedSamplingRotation(prefix string) RotationConfig {
	return c.resolvedRotation(&c.Indices.Sampling, prefix, "", "")
}

func (c *Configuration) resolvedRotation(idxOpts *IndexOptions, prefix, explicitReadAlias, explicitWriteAlias string) RotationConfig {
	if idxOpts.Rotation.HasRotation() {
		return idxOpts.Rotation
	}
	switch {
	case c.getUseILM() && explicitReadAlias != "" && explicitWriteAlias != "":
		return RotationConfig{
			AutoRollover: configoptional.Some(AutoRolloverRotation{
				ReadAlias:  explicitReadAlias,
				WriteAlias: explicitWriteAlias,
			}),
		}
	case explicitReadAlias != "" && explicitWriteAlias != "":
		return RotationConfig{
			ManualRollover: configoptional.Some(ManualRolloverRotation{
				ReadAlias:  explicitReadAlias,
				WriteAlias: explicitWriteAlias,
			}),
		}
	case c.getUseILM():
		writeSuffix := "write"
		if c.WriteAliasSuffix != "" {
			writeSuffix = c.WriteAliasSuffix
		}
		readSuffix := "read"
		if c.ReadAliasSuffix != "" {
			readSuffix = c.ReadAliasSuffix
		}
		return RotationConfig{
			AutoRollover: configoptional.Some(AutoRolloverRotation{
				ReadAlias:  prefix + readSuffix,
				WriteAlias: prefix + writeSuffix,
			}),
		}
	case c.getUseReadWriteAliases():
		writeSuffix := "write"
		if c.WriteAliasSuffix != "" {
			writeSuffix = c.WriteAliasSuffix
		}
		readSuffix := "read"
		if c.ReadAliasSuffix != "" {
			readSuffix = c.ReadAliasSuffix
		}
		return RotationConfig{
			ManualRollover: configoptional.Some(ManualRolloverRotation{
				ReadAlias:  prefix + readSuffix,
				WriteAlias: prefix + writeSuffix,
			}),
		}
	default:
		return RotationConfig{
			Periodic: configoptional.Some(PeriodicRotation{
				DateLayout:        idxOpts.GetDateLayout(),
				RolloverFrequency: idxOpts.GetRolloverFrequency(),
			}),
		}
	}
}

// LogDeprecationWarnings logs warnings for any legacy flags that were explicitly set,
// advising users to migrate to the new rotation config.
func (c *Configuration) LogDeprecationWarnings(logger *zap.Logger) {
	type deprecation struct {
		flag      string
		isSet     bool
		migration string
	}
	deprecations := []deprecation{
		{"use_aliases", c.UseReadWriteAliases.HasValue(), "use 'indices.spans.rotation.manual_rollover' (or auto_rollover) instead"},
		{"use_ilm", c.UseILM.HasValue(), "use 'indices.spans.rotation.auto_rollover' instead"},
		{"span_read_alias", c.SpanReadAlias.HasValue(), "use 'indices.spans.rotation.manual_rollover.read_alias' instead"},
		{"span_write_alias", c.SpanWriteAlias.HasValue(), "use 'indices.spans.rotation.manual_rollover.write_alias' instead"},
		{"service_read_alias", c.ServiceReadAlias.HasValue(), "use 'indices.services.rotation.manual_rollover.read_alias' instead"},
		{"service_write_alias", c.ServiceWriteAlias.HasValue(), "use 'indices.services.rotation.manual_rollover.write_alias' instead"},
	}
	for _, d := range deprecations {
		if !d.isSet {
			continue
		}
		logger.Warn(
			"Deprecated Elasticsearch configuration flag",
			zap.String("flag", d.flag),
			zap.String("migration", d.migration),
		)
	}
}
