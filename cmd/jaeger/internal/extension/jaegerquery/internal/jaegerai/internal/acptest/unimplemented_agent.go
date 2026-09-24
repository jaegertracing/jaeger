// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package acptest provides shared building blocks for tests that stand up a
// fake ACP agent for Jaeger's AI client to talk to.
package acptest

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
)

// UnimplementedAgent answers every acp.Agent method with "method not found".
// Test agents embed it and override only the methods their test exercises. The
// SDK adds a required method to acp.Agent whenever one graduates from its
// unstable surface, and embedding this type keeps such a release from breaking
// compilation of stubs that never cared about the new method.
type UnimplementedAgent struct{}

var _ acp.Agent = UnimplementedAgent{}

func (UnimplementedAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, acp.NewMethodNotFound(acp.AgentMethodAuthenticate)
}

func (UnimplementedAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{}, acp.NewMethodNotFound(acp.AgentMethodInitialize)
}

func (UnimplementedAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, acp.NewMethodNotFound(acp.AgentMethodLogout)
}

func (UnimplementedAgent) Cancel(context.Context, acp.CancelNotification) error {
	return acp.NewMethodNotFound(acp.AgentMethodSessionCancel)
}

func (UnimplementedAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionClose)
}

func (UnimplementedAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionList)
}

func (UnimplementedAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionNew)
}

func (UnimplementedAgent) Prompt(context.Context, acp.PromptRequest) (acp.PromptResponse, error) {
	return acp.PromptResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionPrompt)
}

func (UnimplementedAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionResume)
}

func (UnimplementedAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionSetConfigOption)
}

func (UnimplementedAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionSetMode)
}
