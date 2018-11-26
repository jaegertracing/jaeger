package plugin

import (
	"flag"
	"github.com/spf13/viper"
)

// Options stores the configuration entries for this storage
type Options struct {
	//Configuration config.Configuration
}

// AddFlags from this storage to the CLI
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	//flagSet.Int(limit, opt.Configuration.MaxTraces, "The maximum amount of traces to store in memory")
}

// InitFromViper initializes the options struct with values from Viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	//opt.Configuration.MaxTraces = v.GetInt(limit)
}
