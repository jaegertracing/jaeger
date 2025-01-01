package esrollover

import (
	"flag"
	"github.com/spf13/viper"
)

const (
	conditions               = "conditions"
	defaultRollbackCondition = "{\"max_age\": \"2d\"}"
)

type RollBackConditions struct {
	// Conditions store the conditions used to rollover to a new write index
	Conditions string `mapstructure:"conditions"`
}

func (e EsRolloverFlagConfig) AddFlagsForRollBack(flags *flag.FlagSet) {
	flags.String(e.Prefix+conditions, defaultRollbackCondition, "conditions used to rollover to a new write index")
}

func (e EsRolloverFlagConfig) InitFromViperForRollBack(v *viper.Viper) *RollBackConditions {
	r := &RollBackConditions{}
	r.Conditions = v.GetString(e.Prefix+conditions)
	return r
}

func DefaultRollBackConditions() RollBackConditions {
	return RollBackConditions{
		Conditions: defaultRollbackCondition,
	}
}
