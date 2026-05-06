// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
    "context"
    "fmt"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/skills"
    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

type ListSkillsInput struct{}

type ListSkillsOutput struct {
    Skills []skillSummary `json:"skills"`
}

type GetSkillInput struct {
    Name string `json:"name" jsonschema:"description=The unique name of the skill to retrieve."`
}

type GetSkillOutput struct {
    Skill *types.Skill `json:"skill"`
}

type skillSummary struct {
    Name         string   `json:"name"`
    Description  string   `json:"description"`
    AllowedTools []string `json:"allowed_tools"`
    Version      string   `json:"version,omitempty"`
}

type listSkillsHandler struct {
    loader *skills.Loader
}

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
    return nil, ListSkillsOutput{Skills: summaries}, nil
}

type getSkillHandler struct {
    loader *skills.Loader
}

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
