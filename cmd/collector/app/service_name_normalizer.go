// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package app

import "strings"

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
