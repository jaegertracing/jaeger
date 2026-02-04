// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package httperr

import (
	"errors"
	"net/http"
)

// BadRequestError represents a user error (4xx status code)
type BadRequestError struct {
	message string
}

// NewBadRequest creates a new BadRequestError
func NewBadRequest(message string) *BadRequestError {
	return &BadRequestError{message: message}
}

// Error implements the error interface
func (e *BadRequestError) Error() string {
	return e.message
}

// HTTPError represents an HTTP error with status code
type HTTPError struct {
	StatusCode int
	Message    string
}

// NewHTTPError creates a new HTTPError
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{StatusCode: statusCode, Message: message}
}

// Error implements the error interface
func (e *HTTPError) Error() string {
	return e.Message
}

// HandleError checks if the error is not nil, writes it to the output
// with the specified status code, and returns true. If error is nil it returns false.
func HandleError(w http.ResponseWriter, err error, statusCode int) bool {
	if err == nil {
		return false
	}
	http.Error(w, string(err.Error()), statusCode)
	return true
}

// HandleErrorAuto checks if the error is not nil and writes it to the output
// with an appropriate status code based on error type. Returns true if error was handled.
func HandleErrorAuto(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	statusCode := http.StatusInternalServerError

	// Check for HTTPError first (from remote service responses)
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		statusCode = httpErr.StatusCode
	} else {
		// Check for BadRequestError (local validation errors)
		var badReqErr *BadRequestError
		if errors.As(err, &badReqErr) {
			statusCode = http.StatusBadRequest
		}
	}

	http.Error(w, err.Error(), statusCode)
	return true
}
