// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import "context"

// SkillDefinition describes a single skill loaded from static config.
type SkillDefinition struct {
	Name           string         `json:"name" yaml:"name"`
	Version        string         `json:"version,omitempty" yaml:"version,omitempty"`
	SystemPrompt   string         `json:"system_prompt" yaml:"system_prompt"`
	AllowedTools   []string       `json:"allowed_tools" yaml:"allowed_tools"`
	ReasoningHints []string       `json:"reasoning_hints,omitempty" yaml:"reasoning_hints,omitempty"`
	MaxIterations  int            `json:"max_iterations" yaml:"max_iterations"`
	OutputContract OutputContract `json:"output_contract" yaml:"output_contract"`
}

// OutputContract defines the expected output format for a skill.
type OutputContract struct {
	Fields    []string `json:"fields" yaml:"fields"`
	MaxTokens int      `json:"max_tokens" yaml:"max_tokens"`
	Format    string   `json:"format" yaml:"format"`
}

// Backward-compatible alias for existing parser/registry scaffolding.
type Definition = SkillDefinition

// Tool represents an optional tool declaration a skill may use.
type Tool struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description" yaml:"description"`
	Config      map[string]string `json:"config,omitempty" yaml:"config,omitempty"`
}

// WorkflowStep represents one deterministic step in a skill workflow.
type WorkflowStep struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Action      string `json:"action" yaml:"action"`
}

// LocalLLMConfig captures local-first runtime defaults.
type LocalLLMConfig struct {
	Provider string `json:"provider" yaml:"provider"`
	Endpoint string `json:"endpoint" yaml:"endpoint"`
	Model    string `json:"model,omitempty" yaml:"model,omitempty"`
}

// ExecutorConfig captures placeholder integration points for AI execution.
type ExecutorConfig struct {
	Orchestrator string         `json:"orchestrator" yaml:"orchestrator"`
	LocalLLM     LocalLLMConfig `json:"local_llm" yaml:"local_llm"`
}

// DefaultExecutorConfig returns local-first defaults.
// If no external key is configured, it explicitly targets a local Ollama endpoint.
func DefaultExecutorConfig() ExecutorConfig {
	cfg := ExecutorConfig{
		Orchestrator: "langchaingo",
		LocalLLM: LocalLLMConfig{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
		},
	}

	return cfg
}

// RuntimeSkillLoader defines dynamic discovery and loading behavior.
type RuntimeSkillLoader interface {
	Load(ctx context.Context) ([]Definition, error)
}

// RuntimeSkillExecutor defines a placeholder reasoning loop contract.
type RuntimeSkillExecutor interface {
	Execute(ctx context.Context, skillName string, input map[string]any) (map[string]any, error)
}
