// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	logLevel    = "log-level"
	logEncoding = "log-encoding" // json or console
	configFile  = "config-file"
)

// AddConfigFileFlag adds flags for ExternalConfFlags
func AddConfigFileFlag(flagSet *flag.FlagSet) {
	flagSet.String(configFile, "", "Path to the YAML configuration file (default none).")
}

// ConfigFile returns the path given with --config-file, or an empty string when none was.
func ConfigFile(v *viper.Viper) string {
	return v.GetString(configFile)
}

// ParseJaegerTags parses the Jaeger tags string into a map.
func ParseJaegerTags(jaegerTags string) (map[string]string, error) {
	if jaegerTags == "" {
		return nil, nil
	}
	tagPairs := strings.Split(string(jaegerTags), ",")
	tags := make(map[string]string)
	for _, p := range tagPairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid Jaeger tag pair %q, expected key=value", p)
		}
		k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])

		if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
			skipWhenEmpty := false

			ed := strings.SplitN(string(v[2:len(v)-1]), ":", 2)
			if len(ed) == 1 {
				// no default value specified, set to empty
				skipWhenEmpty = true
				ed = append(ed, "")
			}

			e, d := ed[0], ed[1]
			v = os.Getenv(e)
			if v == "" && d != "" {
				v = d
			}

			// no value is set, skip this entry
			if v == "" && skipWhenEmpty {
				continue
			}
		}

		tags[k] = v
	}

	return tags, nil
}

// LoggingConfig is the logging section of a service's configuration file.
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
