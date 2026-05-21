// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// SkillMetadata provides standardized structured metadata for AI skill outputs.
// This allows the AI assistant to track which tools were invoked and the
// specific evidence (like trace or span IDs) that contributed to the result.
type SkillMetadata struct {
	SkillName     string   `json:"_skill_name"`
	EvidenceSpans []string `json:"_evidence_spans"`
}
