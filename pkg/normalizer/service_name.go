// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package normalizer

import (
	"strings"
)

// ServiceName converts service name to a lowercase string that is safe to use in metrics
func ServiceName(serviceName string) string {
	return serviceNameReplacer.Replace(serviceName)
}

var serviceNameReplacer = newServiceNameReplacer()

// Only allowed runes: [a-zA-Z0-9_:-.]
func newServiceNameReplacer() *strings.Replacer {
	var mapping [256]byte
	// we start with everything being replaces with underscore, and later fix some safe characters
	for i := range mapping {
		mapping[i] = '_'
	}
	// digits are safe
	for i := '0'; i <= '9'; i++ {
		mapping[i] = byte(i)
	}
	// lower case letters are safe
	for i := 'a'; i <= 'z'; i++ {
		mapping[i] = byte(i)
	}
	// upper case letters are safe, but convert them to lower case
	for i := 'A'; i <= 'Z'; i++ {
		mapping[i] = byte(i - 'A' + 'a')
	}
	// dash and dot are safe
	mapping['-'] = '-'
	mapping['.'] = '.'

	// prepare array of pairs of bad/good characters
	oldnew := make([]string, 0, 2*(256-2-10-int('z'-'a'+1)))
	for i := range mapping {
		if mapping[i] != byte(i) {
			oldnew = append(oldnew, string(rune(i)), string(rune(mapping[i])))
		}
	}

	return strings.NewReplacer(oldnew...)
}
