package badgercleaner

import (
	"github.com/asaskevich/govalidator"
)

type Config struct {
	TraceStorage string `valid:"required" mapstructure:"trace_storage"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
