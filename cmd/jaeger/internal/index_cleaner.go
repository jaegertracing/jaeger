package internal

import (
	"log"

	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/spf13/viper"
)

// StartCleaner runs the index cleaner if enabled and handles config loading
func StartCleaner(v *viper.Viper) {
	var cleanerConfig config.Cleaner
	if err := v.UnmarshalKey("cleaner", &cleanerConfig); err != nil {
		log.Printf("Error loading cleaner configuration: %v", err)
		return
	}

	cfg := &config.Configuration{
		Cleaner: cleanerConfig,
	}

	// If cleaner is enabled, run the cleaner
	if cfg.Cleaner.Enabled {
		go cfg.RunCleaner()
	}
}
