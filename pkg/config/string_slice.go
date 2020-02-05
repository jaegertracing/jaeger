// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import "strings"

// StringSlice implements the pflag.Value interface and allows for parsing multiple
//  config values with the same name
// It purposefully mimics pFlag.stringSliceValue (https://github.com/spf13/pflag/blob/master/string_slice.go)
//  in order to be treated like a string slice by both viper and pflag cleanly
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
func (l *StringSlice) Type() string {
	// this type string needs to match pflag.stringSliceValue's Type
	return "stringSlice"
}
