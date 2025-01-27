package agent

import (
    modelv1 "github.com/jaegertracing/jaeger-idl/thrift-gen/agent"
)

type Agent = modelv1.Agent

type AgentClient = modelv1.AgentClient
var NewAgentClientFactory = modelv1.NewAgentClientFactory
var NewAgentClientProtocol = modelv1.NewAgentClientProtocol
var NewAgentClient = modelv1.NewAgentClient

type AgentProcessor = modelv1.AgentProcessor
var NewAgentProcessor = modelv1.NewAgentProcessor

type AgentEmitZipkinBatchArgs = modelv1.AgentEmitZipkinBatchArgs
var NewAgentEmitZipkinBatchArgs = modelv1.NewAgentEmitZipkinBatchArgs

type AgentEmitBatchArgs = modelv1.AgentEmitBatchArgs
var NewAgentEmitBatchArgs = modelv1.NewAgentEmitBatchArgs


var GoUnusedProtection__ int