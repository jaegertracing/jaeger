package esrollover

import (
	"flag"
	"github.com/spf13/viper"
)

const (
	unit             = "unit"
	unitCount        = "unit-count"
	defaultUnit      = "days"
	defaultUnitCount = 1
)

type LookBackOptions struct {
	// Unit is used with lookback to remove indices from read alias e.g, days, weeks, months, years
	Unit string `mapstructure:"unit"`
	// UnitCount is the count of units for which look-up is performed
	UnitCount int `mapstructure:"unit-count"`
}

func (e EsRolloverFlagConfig) AddFlagsForLookBack(flags *flag.FlagSet) {
	flags.String(e.Prefix+unit, defaultUnit, "used with lookback to remove indices from read alias e.g, days, weeks, months, years")
	flags.Int(e.Prefix+unitCount, defaultUnitCount, "count of UNITs")
}

func (e EsRolloverFlagConfig) InitFromViperForLookBack(v *viper.Viper) *LookBackOptions {
	l := &LookBackOptions{}
	l.Unit = v.GetString(e.Prefix + unit)
	l.UnitCount = v.GetInt(e.Prefix + unitCount)
	return l
}

func DefaultLookBackOptions() LookBackOptions {
	return LookBackOptions{
		Unit:      defaultUnit,
		UnitCount: defaultUnitCount,
	}
}
