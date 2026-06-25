// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import "testing"

func TestParseSkillYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		yaml      string
		shouldErr bool
	}{
		{
			name: "valid YAML",
			yaml: `
skills:
  - name: trace_explainer
    version: "1.0.0"
    system_prompt: "Explain trace behavior"
    allowed_tools: ["query_traces"]
    reasoning_hints: ["focus on latency"]
    max_iterations: 3
    output_contract:
      fields: ["summary", "actions"]
      max_tokens: 512
      format: "json"
`,
		},
		{
			name: "missing name",
			yaml: `
skills:
  - version: "1.0.0"
    system_prompt: "Explain trace behavior"
    allowed_tools: ["query_traces"]
    max_iterations: 3
    output_contract:
      fields: ["summary"]
      max_tokens: 512
      format: "json"
`,
			shouldErr: true,
		},
		{
			name: "missing system_prompt",
			yaml: `
skills:
  - name: trace_explainer
    version: "1.0.0"
    allowed_tools: ["query_traces"]
    max_iterations: 3
    output_contract:
      fields: ["summary"]
      max_tokens: 512
      format: "json"
`,
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			defs, err := ParseDefinitions("skills.yaml", []byte(tt.yaml))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			err = ValidateDefinitions(defs)
			if tt.shouldErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Fatalf("expected no validation error, got %v", err)
			}
		})
	}
}

func TestValidateSkill(t *testing.T) {
	t.Parallel()

	validSkill := Definition{
		Name:          "trace_explainer",
		Version:       "1.0.0",
		SystemPrompt:  "Explain a trace execution path.",
		AllowedTools:  []string{"query_traces"},
		MaxIterations: 3,
		OutputContract: OutputContract{
			Fields:    []string{"summary", "actions"},
			MaxTokens: 512,
			Format:    "json",
		},
	}

	tests := []struct {
		name  string
		skill Definition
		valid bool
	}{
		{
			name:  "valid skill",
			skill: validSkill,
			valid: true,
		},
		{
			name: "empty allowed_tools",
			skill: Definition{
				Name:          validSkill.Name,
				Version:       validSkill.Version,
				SystemPrompt:  validSkill.SystemPrompt,
				AllowedTools:  []string{},
				MaxIterations: validSkill.MaxIterations,
				OutputContract: OutputContract{
					Fields:    []string{"summary"},
					MaxTokens: 256,
					Format:    "json",
				},
			},
		},
		{
			name: "name with uppercase",
			skill: Definition{
				Name:          "TraceExplainer",
				Version:       validSkill.Version,
				SystemPrompt:  validSkill.SystemPrompt,
				AllowedTools:  []string{"query_traces"},
				MaxIterations: validSkill.MaxIterations,
				OutputContract: OutputContract{
					Fields:    []string{"summary"},
					MaxTokens: 256,
					Format:    "json",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateDefinition(tt.skill)
			if tt.valid && err != nil {
				t.Fatalf("expected valid skill, got %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatalf("expected invalid skill, got nil")
			}
		})
	}
}

func TestValidateDefinitions(t *testing.T) {
	t.Parallel()

	validSkill := Definition{
		Name:          "trace_explainer",
		Version:       "1.0.0",
		SystemPrompt:  "Explain a trace execution path.",
		AllowedTools:  []string{"query_traces"},
		MaxIterations: 3,
		OutputContract: OutputContract{
			Fields:    []string{"summary", "actions"},
			MaxTokens: 512,
			Format:    "json",
		},
	}

	tests := []struct {
		name    string
		defs    []Definition
		wantErr bool
	}{
		{
			name:    "duplicate skill names",
			defs:    []Definition{validSkill, validSkill},
			wantErr: true,
		},
		{
			name: "name with leading whitespace",
			defs: []Definition{{
				Name:           " trace_explainer",
				Version:        validSkill.Version,
				SystemPrompt:   validSkill.SystemPrompt,
				AllowedTools:   validSkill.AllowedTools,
				MaxIterations:  validSkill.MaxIterations,
				OutputContract: validSkill.OutputContract,
			}},
			wantErr: true,
		},
		{
			name: "name with trailing whitespace",
			defs: []Definition{{
				Name:           "trace_explainer ",
				Version:        validSkill.Version,
				SystemPrompt:   validSkill.SystemPrompt,
				AllowedTools:   validSkill.AllowedTools,
				MaxIterations:  validSkill.MaxIterations,
				OutputContract: validSkill.OutputContract,
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateDefinitions(tt.defs)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no validation error, got %v", err)
			}
		})
	}
}
