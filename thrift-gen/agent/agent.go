// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/thrift-gen/agent"
)

type Agent = modelv1.Agent

type AgentClient = modelv1.AgentClient

var (
	NewAgentClientFactory  = modelv1.NewAgentClientFactory
	NewAgentClientProtocol = modelv1.NewAgentClientProtocol
	NewAgentClient         = modelv1.NewAgentClient
)

type AgentProcessor = modelv1.AgentProcessor

var NewAgentProcessor = modelv1.NewAgentProcessor

type AgentEmitZipkinBatchArgs = modelv1.AgentEmitZipkinBatchArgs

var NewAgentEmitZipkinBatchArgs = modelv1.NewAgentEmitZipkinBatchArgs

type AgentEmitBatchArgs = modelv1.AgentEmitBatchArgs

var NewAgentEmitBatchArgs = modelv1.NewAgentEmitBatchArgs
