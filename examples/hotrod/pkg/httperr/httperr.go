// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package httperr

import (
	"net/http"
)

// HandleError checks if the error is not nil, writes it to the output
// with the specified status code, and returns true. If error is nil it returns false.
func HandleError(w http.ResponseWriter, err error, statusCode int) bool {
	if err == nil {
		return false
	}
	http.Error(w, string(err.Error()), statusCode)
	return true
}
