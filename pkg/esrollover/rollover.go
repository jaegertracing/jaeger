package esrollover

import (
	"flag"
	"github.com/spf13/viper"
)

const (
	archive                 = "archive"
	ilmPolicyName           = "es.ilm-policy-name"
	timeout                 = "timeout"
	skipDependencies        = "skip-dependencies"
	adaptiveSampling        = "adaptive-sampling"
	defaultArchiveValue     = false
	defaultIlmPolicyName    = "jaeger-ilm-policy"
	defaultTimeout          = 120
	defaultSkipDependencies = false
	defaultAdaptiveSampling = false
)

type RolloverOptions struct {
	// Archive if set to true will handle archive indices also
	Archive bool `mapstructure:"archive"`
	// The name of the ILM policy to use if ILM is active
	ILMPolicyName string `mapstructure:"ilm_policy_name"`
	// This stores number of seconds to wait for master node response. By default, it is set to 120
	Timeout int `mapstructure:"timeout"`
	// SkipDependencies if set to true will disable rollover for dependencies index
	SkipDependencies bool `mapstructure:"skip_dependencies"`
	// AdaptiveSampling if set to true will enable rollover for adaptive sampling index
	AdaptiveSampling bool `mapstructure:"adaptive_sampling"`
}

func (e EsRolloverFlagConfig) AddFlagsForRolloverOptions(flags *flag.FlagSet) {
	flags.Bool(e.Prefix+archive, defaultArchiveValue, "Handle archive indices")
	flags.String(e.Prefix+ilmPolicyName, defaultIlmPolicyName, "The name of the ILM policy to use if ILM is active")
	flags.Int(e.Prefix+timeout, defaultTimeout, "Number of seconds to wait for master node response")
	flags.Bool(e.Prefix+skipDependencies, defaultSkipDependencies, "Disable rollover for dependencies index")
	flags.Bool(e.Prefix+adaptiveSampling, defaultAdaptiveSampling, "Enable rollover for adaptive sampling index")
}

func (e EsRolloverFlagConfig) InitFromViperForRolloverOptions(v *viper.Viper) *RolloverOptions {
	r := &RolloverOptions{}
	r.Archive = v.GetBool(e.Prefix+archive)
	r.ILMPolicyName = v.GetString(e.Prefix+ilmPolicyName)
	r.Timeout = v.GetInt(e.Prefix+timeout)
	r.SkipDependencies = v.GetBool(e.Prefix+skipDependencies)
	r.AdaptiveSampling = v.GetBool(e.Prefix+adaptiveSampling)
	return r
}

func DefaultRolloverOptions() RolloverOptions {
	return RolloverOptions{
		Archive:          defaultArchiveValue,
		ILMPolicyName:    defaultIlmPolicyName,
		Timeout:          defaultTimeout,
		SkipDependencies: defaultSkipDependencies,
		AdaptiveSampling: defaultAdaptiveSampling,
	}
}
