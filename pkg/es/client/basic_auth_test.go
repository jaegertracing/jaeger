// Copyright (c) 2021 The Jaeger Authors.
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

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicAuth(t *testing.T) {
	tests := []struct {
		name           string
		username       string
		password       string
		expectedResult string
	}{
		{
			name:           "user and password",
			username:       "admin",
			password:       "qwerty123456",
			expectedResult: "YWRtaW46cXdlcnR5MTIzNDU2",
		},
		{
			name:           "username empty",
			username:       "",
			password:       "qwerty123456",
			expectedResult: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := BasicAuth(test.username, test.password)
			assert.Equal(t, test.expectedResult, result)
		})
	}
}
