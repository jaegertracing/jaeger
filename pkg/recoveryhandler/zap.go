// Copyright (c) 2017 Uber Technologies, Inc.
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
func (z zapRecoveryWrapper) Println(fields ...interface{}) {
	// if you think i'm going to check the type of each of the fields and then logger with fields, you're crazy.
	z.logger.Error(fmt.Sprintln(fields))
}

// NewRecoveryHandler returns an http.Handler that recovers on panics
func NewRecoveryHandler(logger *zap.Logger, printStack bool) func(h http.Handler) http.Handler {
	zWrapper := zapRecoveryWrapper{logger}
	return handlers.RecoveryHandler(handlers.RecoveryLogger(zWrapper), handlers.PrintRecoveryStack(printStack))
}
