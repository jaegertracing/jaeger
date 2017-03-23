// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package recoveryhandler

import (
	"fmt"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/uber-go/zap"
)

// zapRecoveryWrapper wraps a zap logger into a gorilla RecoveryLogger
type zapRecoveryWrapper struct {
	logger zap.Logger
}

// Println logs an error message with the given fields
func (z zapRecoveryWrapper) Println(fields ...interface{}) {
	// if you think i'm going to check the type of each of the fields and then logger with fields, you're crazy.
	z.logger.Error(fmt.Sprintln(fields))
}

// NewRecoveryHandler returns an http.Handler that recovers on panics
func NewRecoveryHandler(logger zap.Logger, printStack bool) func(h http.Handler) http.Handler {
	zWrapper := zapRecoveryWrapper{logger}
	return handlers.RecoveryHandler(handlers.RecoveryLogger(zWrapper), handlers.PrintRecoveryStack(printStack))
}
