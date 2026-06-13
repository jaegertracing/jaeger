// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import "encoding/base64"

// BasicAuth encode username and password to be used with basic authentication header
func BasicAuth(username, password string) string {
	if username == "" || password == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
