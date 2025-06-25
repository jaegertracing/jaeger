// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/confmap"
)

// Viperize creates new Viper and command add passes flags to command
// Viper is initialized with flags from command and configured to accept flags as environmental variables.
// Characters `.-` in environmental variables are changed to `_`
func Viperize(inits ...func(*flag.FlagSet)) (*viper.Viper, *cobra.Command) {
	return AddFlags(viper.New(), &cobra.Command{}, inits...)
}

// AddFlags adds flags to command and viper and configures
func AddFlags(v *viper.Viper, command *cobra.Command, inits ...func(*flag.FlagSet)) (*viper.Viper, *cobra.Command) {
	flagSet := new(flag.FlagSet)
	for i := range inits {
		inits[i](flagSet)
	}
	command.Flags().AddGoFlagSet(flagSet)

	configureViper(v)
	v.BindPFlags(command.Flags())
	return v, command
}

func configureViper(v *viper.Viper) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
}

// ProcessOptionalPointers processes a configuration struct using reflection to properly
// handle optional pointer fields based on whether they were set in the configuration map.
//
// This is a generic solution for the OpenTelemetry Collector optional fields issue:
// https://github.com/open-telemetry/opentelemetry-collector/issues/10266
//
// The function modifies the config struct in-place, setting pointer fields to nil
// if they were not explicitly set in the configuration, even if they have default values.
//
// Parameters:
//   - config: Pointer to the configuration struct to process
//   - conf: The confmap.Conf containing the raw configuration data
//   - fieldPath: Optional prefix for nested field paths (used in recursive calls)
//
// Example usage:
//
//	type Config struct {
//		Required    string      `mapstructure:"required"`
//		Optional    *SubConfig  `mapstructure:"optional"`
//		AnotherOpt  *OtherConf  `mapstructure:"another_opt"`
//	}
//
//	// After normal unmarshal:
//	err := conf.Unmarshal(&config)
//	if err != nil {
//		return err
//	}
//
//	// Process optional fields:
//	err = config.ProcessOptionalPointers(&config, conf, "")
//	if err != nil {
//		return err
//	}
//
//	// Now config.Optional will be nil if "optional" was not in the YAML/config
func ProcessOptionalPointers(config interface{}, conf *confmap.Conf, fieldPath string) error {
	if config == nil {
		return errors.New("config cannot be nil")
	}

	v := reflect.ValueOf(config)
	if v.Kind() != reflect.Ptr {
		return errors.New("config must be a pointer to struct")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return errors.New("config must be a pointer to struct")
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Get the mapstructure tag for the field name
		tag := fieldType.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}

		// Handle squash tag (embedded structs)
		if strings.Contains(tag, "squash") {
			if field.Kind() == reflect.Struct {
				// Recursively process embedded struct
				err := ProcessOptionalPointers(field.Addr().Interface(), conf, fieldPath)
				if err != nil {
					return fmt.Errorf("failed to process embedded struct %s: %w", fieldType.Name, err)
				}
			}
			continue
		}

		// Extract field name from tag (handle omitempty, etc.)
		fieldName := strings.Split(tag, ",")[0]
		if fieldName == "" {
			fieldName = fieldType.Name
		}

		// Process pointer fields that could be optional
		if field.Kind() == reflect.Ptr {
			// Check if the field was explicitly set in config
			if !conf.IsSet(fieldName) {
				// Field was not set in config, set to nil regardless of current value
				field.Set(reflect.Zero(field.Type()))
			} else {
				// Field was explicitly set, check if it was set to null explicitly
				rawValue := conf.Get(fieldName)
				if rawValue == nil {
					// Field was explicitly set to null, keep it as nil
					field.Set(reflect.Zero(field.Type()))
				} else if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
					// Field was set to a value and is a struct, recursively process
					if subConf, err := conf.Sub(fieldName); err == nil {
						err := ProcessOptionalPointers(field.Interface(), subConf, "")
						if err != nil {
							return fmt.Errorf("failed to process nested struct %s: %w", fieldName, err)
						}
					}
				}
				// If field is nil but was not explicitly set to null, it means
				// unmarshal failed for some reason, keep current state
			}
		} else if field.Kind() == reflect.Struct {
			// For non-pointer structs, recursively process nested fields if the section exists
			if conf.IsSet(fieldName) {
				if subConf, err := conf.Sub(fieldName); err == nil {
					err := ProcessOptionalPointers(field.Addr().Interface(), subConf, "")
					if err != nil {
						return fmt.Errorf("failed to process nested struct %s: %w", fieldName, err)
					}
				}
			}
		}
	}

	return nil
}

// IsOptionalFieldSet checks if a specific optional field was explicitly set in the configuration.
// This is a utility function for manual checking when automatic processing is not suitable.
func IsOptionalFieldSet(conf *confmap.Conf, fieldPath string) bool {
	return conf.IsSet(fieldPath)
}
