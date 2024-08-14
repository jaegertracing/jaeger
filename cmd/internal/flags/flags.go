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
	spanStorageType = "span-storage.type" // deprecated
	logLevel        = "log-level"
	logEncoding     = "log-encoding" // json or console
	configFile      = "config-file"
)

// AddConfigFileFlag adds flags for ExternalConfFlags
func AddConfigFileFlag(flagSet *flag.FlagSet) {
	flagSet.String(configFile, "", "Configuration file in JSON, TOML, YAML, HCL, or Java properties formats (default none). See spf13/viper for precedence.")
}

// TryLoadConfigFile initializes viper with config file specified as flag
func TryLoadConfigFile(v *viper.Viper) error {
	if file := v.GetString(configFile); file != "" {
		v.SetConfigFile(file)
		err := v.ReadInConfig()
		if err != nil {
			return fmt.Errorf("cannot load config file %s: %w", file, err)
		}
	}
	return nil
}

// ParseJaegerTags parses the Jaeger tags string into a map.
func ParseJaegerTags(jaegerTags string) map[string]string {
	if jaegerTags == "" {
		return nil
	}
	tagPairs := strings.Split(string(jaegerTags), ",")
	tags := make(map[string]string)
	for _, p := range tagPairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			panic(fmt.Sprintf("invalid Jaeger tag pair %q, expected key=value", p))
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

	return tags
}

// SharedFlags holds flags configuration
type SharedFlags struct {
	// Logging holds logging configuration
	Logging logging
}

type logging struct {
	Level    string
	Encoding string
}

// AddFlags adds flags for SharedFlags
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(spanStorageType, "", "(deprecated) please use SPAN_STORAGE_TYPE environment variable. Run this binary with the 'env' command for help.")
	AddLoggingFlags(flagSet)
}

// AddLoggingFlag adds logging flag for SharedFlags
func AddLoggingFlags(flagSet *flag.FlagSet) {
	flagSet.String(logLevel, "info", "Minimal allowed log Level. For more levels see https://github.com/uber-go/zap")
	flagSet.String(logEncoding, "json", "Log encoding. Supported values are 'json' and 'console'.")
}

// InitFromViper initializes SharedFlags with properties from viper
func (flags *SharedFlags) InitFromViper(v *viper.Viper) *SharedFlags {
	flags.Logging.Level = v.GetString(logLevel)
	flags.Logging.Encoding = v.GetString(logEncoding)
	return flags
}

// NewLogger returns logger based on configuration in SharedFlags
func (flags *SharedFlags) NewLogger(conf zap.Config, options ...zap.Option) (*zap.Logger, error) {
	var level zapcore.Level
	err := (&level).UnmarshalText([]byte(flags.Logging.Level))
	if err != nil {
		return nil, err
	}
	conf.Level = zap.NewAtomicLevelAt(level)
	conf.Encoding = flags.Logging.Encoding
	if flags.Logging.Encoding == "console" {
		conf.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}
	return conf.Build(options...)
}
