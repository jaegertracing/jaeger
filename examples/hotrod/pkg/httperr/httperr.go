// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package httperr

import (
	"errors"
	"net/http"
)

// StatusError is an error that also carries an HTTP status code.
type StatusError struct {
	Code int
	Err  error
}

func (e StatusError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying error to support errors.Is and errors.As.
func (e StatusError) Unwrap() error {
	return e.Err
}

// StatusCode returns the HTTP status code.
func (e StatusError) StatusCode() int {
	return e.Code
}

// HandleError checks if the error carries a status code and writes the response.
func HandleError(w http.ResponseWriter, err error, defaultStatusCode int) bool {
	if err == nil {
		return false
	}

	statusCode := defaultStatusCode
	var se interface{ StatusCode() int }
	if errors.As(err, &se) {
		statusCode = se.StatusCode()
	}

	http.Error(w, err.Error(), statusCode)
	return true
}
