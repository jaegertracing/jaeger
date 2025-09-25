// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"time"

	"github.com/asaskevich/govalidator"
)

type Config struct {
	// Protocol is the protocol to use to connect to ClickHouse.
	// Supported values are "native" and "http". Default is "native".
	Protocol string `mapstructure:"protocol" valid:"in(native,http),optional"`
	// Addresses contains a list of ClickHouse server addresses to connect to.
	Addresses []string `mapstructure:"addresses" valid:"required"`
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

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
