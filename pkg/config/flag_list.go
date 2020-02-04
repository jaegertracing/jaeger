package config

import "strings"

// FlagList implements the pflag.Value interface and allows for parsing multiple
//  config values with the same name
// It purposefully mimics pFlag.stringSliceValue (https://github.com/spf13/pflag/blob/master/string_slice.go)
//  in order to be treated like a string slice by both viper and pflag cleanly
type FlagList []string

// String implements pflag.Value
func (l *FlagList) String() string {
	return "[" + strings.Join(*l, ",") + "]"
}

// Set implements pflag.Value
func (l *FlagList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

// Type implements pflag.Value
func (l *FlagList) Type() string {
	// this type string needs to match pflag.stringSliceValue's Type
	return "stringSlice"
}
