// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"go.opentelemetry.io/collector/config/configoptional"
)

const legacyRotationFlagsList = "use_aliases, use_ilm, span_read_alias, span_write_alias, service_read_alias, service_write_alias"

// RotationConfig defines how Jaeger manages index naming and lookup for a given
// index type (spans, services, etc.). Exactly one variant should be set.
// If none is set, legacy flags (use_aliases, use_ilm, etc.) determine behavior.
type RotationConfig struct {
	// Periodic creates a new time-stamped index on a regular schedule (e.g. daily).
	// Jaeger computes the index name from a date suffix and writes directly to it.
	// This is the default strategy when no rotation is explicitly configured.
	Periodic configoptional.Optional[PeriodicRotation] `mapstructure:"periodic"`
	// ManualRollover uses fixed read/write aliases instead of time-stamped indices.
	// An external tool (e.g. jaeger-es-rollover) must create the aliases and trigger
	// rollover; Jaeger only reads from/writes to the configured alias names.
	ManualRollover configoptional.Optional[ManualRolloverRotation] `mapstructure:"manual_rollover"`
	// AutoRollover uses read/write aliases like ManualRollover, but the rollover is
	// triggered automatically by an ILM (Index Lifecycle Management) or ISM (Index
	// State Management) policy attached to the index template. Jaeger embeds the
	// policy name into the template so ES/OpenSearch can manage the lifecycle.
	AutoRollover configoptional.Optional[AutoRolloverRotation] `mapstructure:"auto_rollover"`
	// DataStream uses ES/OpenSearch data streams for append-only span storage.
	// Not yet implemented.
	DataStream configoptional.Optional[DataStreamRotation] `mapstructure:"data_stream"`
}

// PeriodicRotation configures time-stamped index rotation. Jaeger appends a
// formatted date to the index prefix (e.g. "jaeger-span-2024-03-15") and creates
// a new index each time the date rolls over.
type PeriodicRotation struct {
	// DateLayout is the Go time format string for the date suffix.
	// It controls index granularity, e.g.:
	//   "2006-01-02-15" → hourly
	//   "2006-01-02"    → daily
	//   "2006-01"       → monthly
	//   "2006"          → yearly
	// Defaults to "2006-01-02".
	DateLayout string `mapstructure:"date_layout"`
	// RolloverFrequency controls how many indices are scanned during reads.
	// Must match DateLayout granularity: one of "hour", "day", "month", "year".
	// Defaults to "day".
	RolloverFrequency string `mapstructure:"rollover_frequency"`
}

// ManualRolloverRotation configures alias-based rotation managed externally
// (e.g. by the jaeger-es-rollover job or a cron script).
type ManualRolloverRotation struct {
	// ReadAlias is the alias Jaeger queries for read operations.
	// The external tool must point this alias at all indices that should be searchable.
	// Defaults to "<index-prefix>read" (e.g. "jaeger-span-read").
	ReadAlias string `mapstructure:"read_alias"`
	// WriteAlias is the alias Jaeger writes to.
	// The external tool must point this alias at the current active index.
	// Defaults to "<index-prefix>write" (e.g. "jaeger-span-write").
	WriteAlias string `mapstructure:"write_alias"`
}

// AutoRolloverRotation configures alias-based rotation where ES/OpenSearch
// automatically triggers rollover via an ILM/ISM policy.
type AutoRolloverRotation struct {
	// ReadAlias is the alias Jaeger queries for read operations.
	// Defaults to "<index-prefix>read" (e.g. "jaeger-span-read").
	ReadAlias string `mapstructure:"read_alias"`
	// WriteAlias is the alias Jaeger writes to. ES/OpenSearch automatically
	// rolls over to a new backing index when the policy conditions are met.
	// Defaults to "<index-prefix>write" (e.g. "jaeger-span-write").
	WriteAlias string `mapstructure:"write_alias"`
	// PolicyName is embedded into the index template so that ES/OpenSearch
	// attaches this lifecycle policy to every new index created by rollover.
	// If empty, the template is created without a policy reference and the
	// policy must be attached to indices through other means.
	PolicyName string `mapstructure:"policy_name"`
}

// DataStreamRotation configures data stream-based storage (not yet implemented).
type DataStreamRotation struct {
	// PolicyName is embedded into the data stream's index template so that
	// ES/OpenSearch manages the lifecycle (rollover, retention) automatically.
	// If empty, the cluster's default lifecycle policy applies.
	PolicyName string `mapstructure:"policy_name"`
	// PolicyFile is a path to a JSON file containing the ILM/ISM policy definition.
	// When set, Jaeger creates or updates the policy at startup before creating
	// the data stream. If empty, the policy must already exist in the cluster.
	PolicyFile string `mapstructure:"policy_file"`
	// ReadAlias is an optional alias layered on top of the data stream for reads.
	// Useful for cross-cluster search or to decouple consumer queries from the
	// underlying data stream name. If empty, reads go directly to the data stream.
	ReadAlias string `mapstructure:"read_alias"`
}

// HasRotation returns true if any rotation variant is explicitly configured.
func (r *RotationConfig) HasRotation() bool {
	return r.Periodic.HasValue() || r.ManualRollover.HasValue() ||
		r.AutoRollover.HasValue() || r.DataStream.HasValue()
}

func (c *Configuration) validateRotationConfig() error {
	hasAnyRotation := c.Indices.Spans.Rotation.HasRotation() ||
		c.Indices.Services.Rotation.HasRotation() ||
		c.Indices.Dependencies.Rotation.HasRotation() ||
		c.Indices.Sampling.Rotation.HasRotation()

	if hasAnyRotation && c.hasAnyLegacyRotationFlags() {
		return fmt.Errorf(
			"cannot use both 'rotation' config and legacy flags (%s) simultaneously; "+
				"remove the legacy flags and use the 'rotation' section instead",
			legacyRotationFlagsList,
		)
	}

	type rotationEntry struct {
		name     string
		opts     *IndexOptions
		rotation *RotationConfig
	}
	entries := []rotationEntry{
		{"spans", &c.Indices.Spans, &c.Indices.Spans.Rotation},
		{"services", &c.Indices.Services, &c.Indices.Services.Rotation},
		{"dependencies", &c.Indices.Dependencies, &c.Indices.Dependencies.Rotation},
		{"sampling", &c.Indices.Sampling, &c.Indices.Sampling.Rotation},
	}
	for _, e := range entries {
		if err := e.rotation.validate(e.name); err != nil {
			return err
		}
		if e.rotation.HasRotation() && (e.opts.DateLayout.HasValue() || e.opts.RolloverFrequency.HasValue()) {
			return fmt.Errorf(
				"indices.%s: cannot use both 'rotation' config and legacy "+
					"'date_layout'/'rollover_frequency' fields; use 'rotation.periodic' instead",
				e.name,
			)
		}
	}

	return nil
}

func (r *RotationConfig) validate(indexType string) error {
	count := 0
	if r.Periodic.HasValue() {
		count++
	}
	if r.ManualRollover.HasValue() {
		count++
	}
	if r.AutoRollover.HasValue() {
		count++
	}
	if r.DataStream.HasValue() {
		count++
	}
	if count > 1 {
		return fmt.Errorf(
			"indices.%s.rotation: exactly one rotation strategy must be set, found %d",
			indexType, count,
		)
	}
	if r.DataStream.HasValue() {
		return fmt.Errorf(
			"indices.%s.rotation: data_stream is not yet implemented",
			indexType,
		)
	}
	return nil
}
