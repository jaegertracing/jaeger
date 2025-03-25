// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
