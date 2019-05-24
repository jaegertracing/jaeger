// Copyright (c) 2018 The Jaeger Authors.
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

package processor

import (
	"sync"

	"go.uber.org/zap"
)

// ParallelProcessor is a processor that processes in parallel using a pool of goroutines
type ParallelProcessor struct {
	messages    chan message
	processor   SpanProcessor
	numRoutines int
	errors      chan message

	logger *zap.Logger
	closed chan struct{}
	wg     sync.WaitGroup
}

type message struct {
	message Message
	error   error
	onError OnError
}

func (m *message) handleError() {
	if m.onError != nil {
		m.onError(m.message, m.error)
	}
}

// NewParallelProcessor creates a new parallel processor
func NewParallelProcessor(
	processor SpanProcessor,
	parallelism int,
	logger *zap.Logger) *ParallelProcessor {
	return &ParallelProcessor{
		logger:      logger,
		messages:    make(chan message),
		errors:      make(chan message),
		processor:   processor,
		numRoutines: parallelism,
		closed:      make(chan struct{}),
	}
}

// Start begins processing queued messages
func (k *ParallelProcessor) Start() {
	k.logger.Debug("Spawning goroutine to process errors")
	go k.processErrors()

	k.logger.Debug("Spawning goroutines to process messages", zap.Int("num_routines", k.numRoutines))
	for i := 0; i < k.numRoutines; i++ {
		k.wg.Add(1)
		go func() {
			for {
				select {
				case msg := <-k.messages:
					err := k.processor.Process(msg.message)
					if err != nil {
						msg.error = err
						k.errors <- msg
					}
				case <-k.closed:
					k.wg.Done()
					return
				}
			}
		}()
	}
}

// Process queues a message for processing
func (k *ParallelProcessor) Process(msg Message, onError OnError) {
	k.messages <- message{
		message: msg,
		onError: onError,
	}
}

func (k *ParallelProcessor) processErrors() {
	for msg := range k.errors {
		msg.handleError()
	}
}

// Close terminates all running goroutines
func (k *ParallelProcessor) Close() error {
	k.logger.Debug("Initiated shutdown of processor goroutines")
	close(k.closed)
	k.wg.Wait()
	close(k.errors)
	k.logger.Info("Completed shutdown of processor goroutines")
	return nil
}
