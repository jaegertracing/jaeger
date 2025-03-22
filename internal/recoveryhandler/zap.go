// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017-2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package recoveryhandler

import (
	"fmt"
	"net/http"

	"github.com/gorilla/handlers"
	"go.uber.org/zap"
)

// zapRecoveryWrapper wraps a zap logger into a gorilla RecoveryLogger
type zapRecoveryWrapper struct {
	logger *zap.Logger
}

// Println logs an error message with the given fields
func (z zapRecoveryWrapper) Println(args ...any) {
	z.logger.Error(fmt.Sprint(args...))
}

// NewRecoveryHandler returns an http.Handler that recovers on panics
func NewRecoveryHandler(logger *zap.Logger, printStack bool) func(h http.Handler) http.Handler {
	zWrapper := zapRecoveryWrapper{logger}
	return handlers.RecoveryHandler(handlers.RecoveryLogger(zWrapper), handlers.PrintRecoveryStack(printStack))
}
