// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	unit                     = "unit"
	unitCount                = "unit-count"
	defaultUnit              = "days"
	defaultUnitCount         = 1
	conditions               = "conditions"
	defaultRollBackCondition = "{\"max_age\": \"2d\"}"
	archive                  = "archive"
	ilmPolicyName            = "es.ilm-policy-name"
	timeout                  = "timeout"
	skipDependencies         = "skip-dependencies"
	adaptiveSampling         = "adaptive-sampling"
	defaultArchiveValue      = false
	defaultIlmPolicyName     = "jaeger-ilm-policy"
	defaultTimeout           = 120
	defaultSkipDependencies  = false
	defaultAdaptiveSampling  = false
)

type EsRolloverFlagConfig struct {
	Prefix string
}

func (e EsRolloverFlagConfig) AddFlagsForRolloverOptions(flags *flag.FlagSet) {
	flags.Bool(e.Prefix+archive, defaultArchiveValue, "Handle archive indices")
	flags.String(e.Prefix+ilmPolicyName, defaultIlmPolicyName, "The name of the ILM policy to use if ILM is active")
	flags.Int(e.Prefix+timeout, defaultTimeout, "Number of seconds to wait for master node response")
	flags.Bool(e.Prefix+skipDependencies, defaultSkipDependencies, "Disable rollover for dependencies index")
	flags.Bool(e.Prefix+adaptiveSampling, defaultAdaptiveSampling, "Enable rollover for adaptive sampling index")
}

func (e EsRolloverFlagConfig) InitRolloverOptionsFromViper(v *viper.Viper) RolloverOptions {
	r := &RolloverOptions{}
	r.Archive = v.GetBool(e.Prefix + archive)
	r.ILMPolicyName = v.GetString(e.Prefix + ilmPolicyName)
	r.Timeout = v.GetInt(e.Prefix + timeout)
	r.SkipDependencies = v.GetBool(e.Prefix + skipDependencies)
	r.AdaptiveSampling = v.GetBool(e.Prefix + adaptiveSampling)
	return *r
}

func (e EsRolloverFlagConfig) AddFlagsForLookBackOptions(flags *flag.FlagSet) {
	flags.String(e.Prefix+unit, defaultUnit, "used with lookback to remove indices from read alias e.g, days, weeks, months, years")
	flags.Int(e.Prefix+unitCount, defaultUnitCount, "count of UNITs")
}

func (e EsRolloverFlagConfig) InitLookBackFromViper(v *viper.Viper) LookBackOptions {
	l := &LookBackOptions{}
	l.Unit = v.GetString(e.Prefix + unit)
	l.UnitCount = v.GetInt(e.Prefix + unitCount)
	return *l
}

func (e EsRolloverFlagConfig) AddFlagsForRollBackOptions(flags *flag.FlagSet) {
	flags.String(e.Prefix+conditions, defaultRollBackCondition, "conditions used to rollover to a new write index")
}

func (e EsRolloverFlagConfig) InitRollBackFromViper(v *viper.Viper) RollBackOptions {
	r := &RollBackOptions{}
	r.Conditions = v.GetString(e.Prefix + conditions)
	return *r
}
