// Copyright (c) 2022 The Jaeger Authors.
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

package tlscfg

import (
	"crypto/tls"
	"fmt"
	"strings"
)

var (
	// https://golang.org/pkg/crypto/tls/#pkg-constants
	ciphers         = map[string]uint16{}
	insecureCiphers = map[string]uint16{}
)

func init() {
	for _, suite := range tls.CipherSuites() {
		ciphers[suite.Name] = suite.ID
	}
	for _, suite := range tls.InsecureCipherSuites() {
		insecureCiphers[suite.Name] = suite.ID
	}
}

func allCiphers() map[string]uint16 {
	acceptedCiphers := make(map[string]uint16, len(ciphers))
	for k, v := range ciphers {
		acceptedCiphers[k] = v
	}
	for k, v := range insecureCiphers {
		acceptedCiphers[k] = v
	}
	return acceptedCiphers
}

// TLSCipherSuites returns a list of cipher suite IDs from the cipher suite names passed.
func TLSCipherSuites(cipherNames string) ([]uint16, error) {
	if cipherNames == "" {
		return nil, nil
	}
	ciphersIntSlice := make([]uint16, 0)
	possibleCiphers := allCiphers()
	for _, cipher := range strings.Split(cipherNames, ",") {
		intValue, ok := possibleCiphers[cipher]
		if !ok {
			return nil, fmt.Errorf("cipher suite %s not supported or doesn't exist", cipher)
		}
		ciphersIntSlice = append(ciphersIntSlice, intValue)
	}
	return ciphersIntSlice, nil
}
