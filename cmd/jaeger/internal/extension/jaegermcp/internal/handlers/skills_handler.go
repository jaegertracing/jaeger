// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
    "context"
    "fmt"
    "sort"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/skills"
    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

// ListSkillsInput is the input for the list_skills tool (no parameters required).
type ListSkillsInput struct{}

// ListSkillsOutput is the output for the list_skills tool.
type ListSkillsOutput struct {
    Skills []skillSummary `json:"skills"`
}

// GetSkillInput is the input for the get_skill tool.
type GetSkillInput struct {
    Name string `json:"name" jsonschema:"The unique name of the skill to retrieve."`
}

// GetSkillOutput is the output for the get_skill tool.
type GetSkillOutput struct {
    Skill *types.Skill `json:"skill"`
}

// skillSummary is the lightweight representation returned by list_skills.
type skillSummary struct {
    Name         string   `json:"name"`
    Description  string   `json:"description"`
    AllowedTools []string `json:"allowed_tools"`
    Version      string   `json:"version,omitempty"`
}

type listSkillsHandler struct {
    loader *skills.Loader
}

// NewListSkillsHandler creates a new list_skills handler and returns the handler function.
func NewListSkillsHandler(loader *skills.Loader) mcp.ToolHandlerFor[ListSkillsInput, ListSkillsOutput] {
    h := &listSkillsHandler{loader: loader}
    return h.handle
}

func (h *listSkillsHandler) handle(_ context.Context, _ *mcp.CallToolRequest, _ ListSkillsInput) (*mcp.CallToolResult, ListSkillsOutput, error) {
    all := h.loader.All()
    summaries := make([]skillSummary, 0, len(all))
    for _, s := range all {
        summaries = append(summaries, skillSummary{
            Name:         s.Name,
            Description:  s.Description,
            AllowedTools: s.AllowedTools,
            Version:      s.Version,
        })
    }
    sort.Slice(summaries, func(i, j int) bool {
        return summaries[i].Name < summaries[j].Name
    })
    return nil, ListSkillsOutput{Skills: summaries}, nil
}

type getSkillHandler struct {
    loader *skills.Loader
}

// NewGetSkillHandler creates a new get_skill handler and returns the handler function.
func NewGetSkillHandler(loader *skills.Loader) mcp.ToolHandlerFor[GetSkillInput, GetSkillOutput] {
    h := &getSkillHandler{loader: loader}
    return h.handle
}

func (h *getSkillHandler) handle(_ context.Context, _ *mcp.CallToolRequest, input GetSkillInput) (*mcp.CallToolResult, GetSkillOutput, error) {
    if input.Name == "" {
        return nil, GetSkillOutput{}, fmt.Errorf("name is required")
    }
    skill, ok := h.loader.Get(input.Name)
    if !ok {
        return nil, GetSkillOutput{}, fmt.Errorf("skill %q not found; use list_skills to see available skills", input.Name)
    }
    return nil, GetSkillOutput{Skill: skill}, nil
}

