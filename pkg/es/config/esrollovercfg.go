// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	flagLookBackUnit             = "unit"
	flagLookBackUnitCount        = "unit-count"
	flagRolloverConditions       = "conditions"
	flagRolloverArchive          = "archive"
	flagRolloverIlmPolicyName    = "es.ilm-policy-name"
	flagRolloverTimeout          = "timeout"
	flagRolloverSkipDependencies = "skip-dependencies"
	flagRolloverAdaptiveSampling = "adaptive-sampling"

	defaultArchiveValue      = false
	defaultIlmPolicyName     = "jaeger-ilm-policy"
	defaultTimeout           = 120
	defaultSkipDependencies  = false
	defaultAdaptiveSampling  = false
	defaultUnit              = "days"
	defaultUnitCount         = 1
	defaultRollBackCondition = "{\"max_age\": \"2d\"}"
)

type EsRolloverFlagConfig struct {
	Prefix string
}

func (e EsRolloverFlagConfig) AddFlagsForRolloverOptions(flags *flag.FlagSet) {
	flags.Bool(e.Prefix+flagRolloverArchive, defaultArchiveValue, "Handle archive indices")
	flags.String(e.Prefix+flagRolloverIlmPolicyName, defaultIlmPolicyName, "The name of the ILM policy to use if ILM is active")
	flags.Int(e.Prefix+flagRolloverTimeout, defaultTimeout, "Number of seconds to wait for master node response")
	flags.Bool(e.Prefix+flagRolloverSkipDependencies, defaultSkipDependencies, "Disable rollover for dependencies index")
	flags.Bool(e.Prefix+flagRolloverAdaptiveSampling, defaultAdaptiveSampling, "Enable rollover for adaptive sampling index")
}

func (e EsRolloverFlagConfig) InitRolloverOptionsFromViper(v *viper.Viper) RolloverOptions {
	r := &RolloverOptions{}
	r.Archive = v.GetBool(e.Prefix + flagRolloverArchive)
	r.ILMPolicyName = v.GetString(e.Prefix + flagRolloverIlmPolicyName)
	r.Timeout = v.GetInt(e.Prefix + flagRolloverTimeout)
	r.SkipDependencies = v.GetBool(e.Prefix + flagRolloverSkipDependencies)
	r.AdaptiveSampling = v.GetBool(e.Prefix + flagRolloverAdaptiveSampling)
	return *r
}

func (e EsRolloverFlagConfig) AddFlagsForLookBackOptions(flags *flag.FlagSet) {
	flags.String(e.Prefix+flagLookBackUnit, defaultUnit, "used with lookback to remove indices from read alias e.g, days, weeks, months, years")
	flags.Int(e.Prefix+flagLookBackUnitCount, defaultUnitCount, "count of UNITs")
}

func (e EsRolloverFlagConfig) InitLookBackFromViper(v *viper.Viper) LookBackOptions {
	l := &LookBackOptions{}
	l.Unit = v.GetString(e.Prefix + flagLookBackUnit)
	l.UnitCount = v.GetInt(e.Prefix + flagLookBackUnitCount)
	return *l
}

func (e EsRolloverFlagConfig) AddFlagsForRollBackOptions(flags *flag.FlagSet) {
	flags.String(e.Prefix+flagRolloverConditions, defaultRollBackCondition, "conditions used to rollover to a new write index")
}

func (e EsRolloverFlagConfig) InitRollBackFromViper(v *viper.Viper) RollOverConditions {
	r := &RollOverConditions{}
	r.Conditions = v.GetString(e.Prefix + flagRolloverConditions)
	return *r
}
