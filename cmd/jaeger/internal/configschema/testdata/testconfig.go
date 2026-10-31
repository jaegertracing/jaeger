// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

import (
	"time"

	"go.opentelemetry.io/collector/config/configoptional"
)

// TestConfig is a sample component config used by the schema generator tests.
type TestConfig struct {
	// RequiredField is a required string with a default value.
	RequiredField string `mapstructure:"required_field"`

	// OptionalField is an optional string.
	OptionalField string `mapstructure:"optional_field,omitempty"`

	// Nested holds nested settings.
	Nested NestedConfig `mapstructure:"nested"`

	// Duration is a time duration.
	Duration time.Duration `mapstructure:"duration,omitempty"`
}

// NestedConfig is a nested struct used by TestConfig.
type NestedConfig struct {
	// Flag is a boolean flag.
	Flag bool `mapstructure:"flag"`
}

// EdgeCaseConfig exercises the schema generator's edge cases: a squashed
// embedded struct, a map, a slice, a pointer to a named struct, an
// configoptional.Optional value, and a deprecated field.
type EdgeCaseConfig struct {
	// Squashed is embedded with mapstructure:",squash", so its fields are
	// flattened into the parent schema.
	Squashed `mapstructure:",squash"`

	// IntMap is a map of integer values.
	IntMap map[string]int `mapstructure:"int_map,omitempty"`

	// StringSlice is a list of strings.
	StringSlice []string `mapstructure:"string_slice,omitempty"`

	// Target points to a named struct.
	Target *EdgeTarget `mapstructure:"target,omitempty"`

	// MaybeCount is an optional integer.
	MaybeCount configoptional.Optional[int] `mapstructure:"maybe_count,omitempty"`

	// DeprecatedField is no longer used.
	// Deprecated: Use IntMap instead.
	DeprecatedField string `mapstructure:"deprecated_field,omitempty"`
}

// Squashed is a struct embedded with mapstructure:",squash" in EdgeCaseConfig.
type Squashed struct {
	// SquashFlag is a boolean flag that must appear directly in the parent schema.
	SquashFlag bool `mapstructure:"squash_flag"`
}

// EdgeTarget is a named struct referenced by pointer from EdgeCaseConfig.
type EdgeTarget struct {
	// Name is the target name.
	Name string `mapstructure:"name"`
}

// DefaultEdgeCaseConfig returns a populated EdgeCaseConfig for schema generation tests.
func DefaultEdgeCaseConfig() *EdgeCaseConfig {
	return &EdgeCaseConfig{
		Squashed:    Squashed{SquashFlag: true},
		IntMap:      map[string]int{"a": 1},
		StringSlice: []string{"x", "y"},
		Target:      &EdgeTarget{Name: "target"},
	}
}

// DefaultTestConfig returns a populated TestConfig for schema generation tests.
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		RequiredField: "hello",
		Duration:      5 * time.Second,
		Nested:        NestedConfig{Flag: true},
	}
}
