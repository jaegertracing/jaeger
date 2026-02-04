// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package httperr

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		expectCode int
		expectBody string
	}{
		{
			name:       "no error",
			err:        nil,
			statusCode: http.StatusInternalServerError,
			expectCode: http.StatusOK,
			expectBody: "",
		},
		{
			name:       "with error",
			err:        errors.New("test error"),
			statusCode: http.StatusInternalServerError,
			expectCode: http.StatusInternalServerError,
			expectBody: "test error",
		},
		{
			name:       "bad request error",
			err:        errors.New("bad input"),
			statusCode: http.StatusBadRequest,
			expectCode: http.StatusBadRequest,
			expectBody: "bad input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			result := HandleError(w, tt.err, tt.statusCode)

			if tt.err == nil {
				assert.False(t, result)
				assert.Equal(t, http.StatusOK, w.Code)
			} else {
				assert.True(t, result)
				assert.Equal(t, tt.expectCode, w.Code)
				assert.Contains(t, w.Body.String(), tt.expectBody)
			}
		})
	}
}

func TestHandleErrorAuto(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		expectCode int
		expectBody string
	}{
		{
			name:       "no error",
			err:        nil,
			expectCode: http.StatusOK,
			expectBody: "",
		},
		{
			name:       "bad request error",
			err:        NewBadRequest("invalid customer ID"),
			expectCode: http.StatusBadRequest,
			expectBody: "invalid customer ID",
		},
		{
			name:       "http error 400",
			err:        NewHTTPError(http.StatusBadRequest, "invalid input"),
			expectCode: http.StatusBadRequest,
			expectBody: "invalid input",
		},
		{
			name:       "http error 500",
			err:        NewHTTPError(http.StatusInternalServerError, "database error"),
			expectCode: http.StatusInternalServerError,
			expectBody: "database error",
		},
		{
			name:       "generic error",
			err:        errors.New("database connection failed"),
			expectCode: http.StatusInternalServerError,
			expectBody: "database connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			result := HandleErrorAuto(w, tt.err)

			if tt.err == nil {
				assert.False(t, result)
				assert.Equal(t, http.StatusOK, w.Code)
			} else {
				assert.True(t, result)
				assert.Equal(t, tt.expectCode, w.Code)
				assert.Contains(t, w.Body.String(), tt.expectBody)
			}
		})
	}
}

func TestNewBadRequest(t *testing.T) {
	err := NewBadRequest("test message")
	assert.NotNil(t, err)
	assert.Equal(t, "test message", err.Error())
	assert.IsType(t, &BadRequestError{}, err)
}

func TestNewHTTPError(t *testing.T) {
	err := NewHTTPError(http.StatusBadRequest, "test message")
	assert.NotNil(t, err)
	assert.Equal(t, "test message", err.Error())
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.IsType(t, &HTTPError{}, err)
}
