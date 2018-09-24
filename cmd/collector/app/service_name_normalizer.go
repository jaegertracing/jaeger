// Copyright (c) 2017 Uber Technologies, Inc.
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

package app

import (
	"strings"
)

// NormalizeServiceName converts service name to a lowercase string that is safe to use in metrics
func NormalizeServiceName(serviceName string) string {
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
	// upper case latters are safe, but convert them to lower case
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
			oldnew = append(oldnew, string(i), string(mapping[i]))
		}
	}

	return strings.NewReplacer(oldnew...)
}
