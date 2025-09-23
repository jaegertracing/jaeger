package tracestore

import (
	"time"
)

type Config struct {
	// Addresses contains a list of ClickHouse server addresses to connect to.
	Addresses []string `mapstructure:"addresses"`
	// Auth contains the authentication configuration to connect to ClickHouse.
	Auth AuthConfig `mapstructure:"auth"`
	// DialTimeout is the timeout for establishing a connection to ClickHouse.
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
	// TODO: add more settings
}

type AuthConfig struct {
	// Database is the ClickHouse database to connect to.
	Database string `mapstructure:"database"`
	// Username is the username to connect to ClickHouse.
	Username string `mapstructure:"username"`
	// Password is the password to connect to ClickHouse.
	Password string `mapstructure:"password"`
}
