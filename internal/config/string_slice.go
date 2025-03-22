// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import "strings"

// StringSlice implements the pflag.Value interface and allows for parsing multiple
// config values with the same name. It purposefully mimics pFlag.stringSliceValue
// (https://github.com/spf13/pflag/blob/master/string_slice.go) in order to be
// treated like a string slice by both viper and pflag cleanly.
type StringSlice []string

// String implements pflag.Value
func (l *StringSlice) String() string {
	if len(*l) == 0 {
		return "[]"
	}

	return `["` + strings.Join(*l, `","`) + `"]`
}

// Set implements pflag.Value
func (l *StringSlice) Set(value string) error {
	*l = append(*l, value)
	return nil
}

// Type implements pflag.Value
func (*StringSlice) Type() string {
	// this type string needs to match pflag.stringSliceValue's Type
	return "stringSlice"
}
