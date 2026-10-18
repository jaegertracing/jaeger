// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LoggingConfig is the logging section of the configuration file.
type LoggingConfig struct {
	// Level is the minimal level a log line needs to be emitted; see go.uber.org/zap for the levels.
	Level string `mapstructure:"level"`
	// Encoding is the log line format, "json" or "console".
	Encoding string `mapstructure:"encoding"`
}

// NewLogger returns a logger built from conf with the level and encoding configured here.
func (c LoggingConfig) NewLogger(conf zap.Config, options ...zap.Option) (*zap.Logger, error) {
	var level zapcore.Level
	err := (&level).UnmarshalText([]byte(c.Level))
	if err != nil {
		return nil, err
	}
	conf.Level = zap.NewAtomicLevelAt(level)
	conf.Encoding = c.Encoding
	if c.Encoding == "console" {
		conf.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}
	return conf.Build(options...)
}
