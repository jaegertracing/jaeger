// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package configschema extracts and documents Jaeger-v2 configuration structures.
//
// It uses Go's reflection and AST parsing to extract:
//   - Field names and types
//   - JSON tag names
//   - Field comments from source code
//   - Default values
//   - Required field indicators
//
// Usage:
//
//	jaeger generate-config-schema --output=config.json
package configschema
