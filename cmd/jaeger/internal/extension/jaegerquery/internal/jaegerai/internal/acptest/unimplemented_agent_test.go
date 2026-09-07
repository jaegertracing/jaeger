// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package acptest

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnimplementedAgent(t *testing.T) {
	var agent UnimplementedAgent
	ctx := t.Context()

	calls := map[string]func() error{
		acp.AgentMethodAuthenticate: func() error {
			_, err := agent.Authenticate(ctx, acp.AuthenticateRequest{})
			return err
		},
		acp.AgentMethodInitialize: func() error {
			_, err := agent.Initialize(ctx, acp.InitializeRequest{})
			return err
		},
		acp.AgentMethodLogout: func() error {
			_, err := agent.Logout(ctx, acp.LogoutRequest{})
			return err
		},
		acp.AgentMethodSessionCancel: func() error {
			return agent.Cancel(ctx, acp.CancelNotification{})
		},
		acp.AgentMethodSessionClose: func() error {
			_, err := agent.CloseSession(ctx, acp.CloseSessionRequest{})
			return err
		},
		acp.AgentMethodSessionList: func() error {
			_, err := agent.ListSessions(ctx, acp.ListSessionsRequest{})
			return err
		},
		acp.AgentMethodSessionNew: func() error {
			_, err := agent.NewSession(ctx, acp.NewSessionRequest{})
			return err
		},
		acp.AgentMethodSessionPrompt: func() error {
			_, err := agent.Prompt(ctx, acp.PromptRequest{})
			return err
		},
		acp.AgentMethodSessionResume: func() error {
			_, err := agent.ResumeSession(ctx, acp.ResumeSessionRequest{})
			return err
		},
		acp.AgentMethodSessionSetConfigOption: func() error {
			_, err := agent.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{})
			return err
		},
		acp.AgentMethodSessionSetMode: func() error {
			_, err := agent.SetSessionMode(ctx, acp.SetSessionModeRequest{})
			return err
		},
	}

	for method, call := range calls {
		t.Run(method, func(t *testing.T) {
			var reqErr *acp.RequestError
			require.ErrorAs(t, call(), &reqErr)
			assert.Equal(t, acp.NewMethodNotFound(method), reqErr)
		})
	}
}
