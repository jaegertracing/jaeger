package dependencyprocessor

import "time"

type Config struct {
	AggregationInterval time.Duration `yaml:"aggregation_interval"`
	InactivityTimeout   time.Duration `yaml:"inactivity_timeout"`
}

func DefaultConfig() Config {
	return Config{
		AggregationInterval: 5 * time.Second, // 默认每5秒聚合一次依赖
		InactivityTimeout:   2 * time.Second, // 默认trace不活跃2秒后视为完成
	}
}
