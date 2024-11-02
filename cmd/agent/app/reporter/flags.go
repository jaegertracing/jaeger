// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/all-in-one/setupcontext"
	"github.com/jaegertracing/jaeger/cmd/internal/flags"
)

const (
	// Reporter type
	reporterType = "reporter.type"
	// GRPC is name of gRPC reporter.
	GRPC Type = "grpc"

	agentTags = "agent.tags"
)

// Type defines type of reporter.
type Type string

// Options holds generic reporter configuration.
type Options struct {
	ReporterType Type
	AgentTags    map[string]string
}

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.String(reporterType, string(GRPC), fmt.Sprintf("Reporter type to use e.g. %s", string(GRPC)))
	if !setupcontext.IsAllInOne() {
		flags.String(agentTags, "", "One or more tags to be added to the Process tags of all spans passing through this agent. Ex: key1=value1,key2=${envVar:defaultValue}")
	}
}

// InitFromViper initializes Options with properties retrieved from Viper.
func (b *Options) InitFromViper(v *viper.Viper, _ *zap.Logger) *Options {
	b.ReporterType = Type(v.GetString(reporterType))
	if !setupcontext.IsAllInOne() {
		if len(v.GetString(agentTags)) > 0 {
			b.AgentTags = flags.ParseJaegerTags(v.GetString(agentTags))
		}
	}
	return b
}
