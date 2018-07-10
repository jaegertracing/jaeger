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
	messages    chan Message
	processor   SpanProcessor
	numRoutines int

	logger *zap.Logger
	closed chan struct{}
	wg     sync.WaitGroup
}

// NewParallelProcessor creates a new parallel processor
func NewParallelProcessor(
	processor SpanProcessor,
	parallelism int,
	logger *zap.Logger) *ParallelProcessor {
	return &ParallelProcessor{
		logger:      logger,
		messages:    make(chan Message),
		processor:   processor,
		numRoutines: parallelism,
		closed:      make(chan struct{}),
	}
}

// Start begins processing queued messages
func (k *ParallelProcessor) Start() {
	k.logger.Debug("Spawning goroutines to process messages", zap.Int("num_routines", k.numRoutines))
	for i := 0; i < k.numRoutines; i++ {
		k.wg.Add(1)
		go func() {
			for {
				select {
				case msg := <-k.messages:
					k.processor.Process(msg)
				case <-k.closed:
					k.wg.Done()
					return
				}
			}
		}()
	}
}

// Process queues a message for processing
func (k *ParallelProcessor) Process(message Message) error {
	k.messages <- message
	return nil
}

// Close terminates all running goroutines
func (k *ParallelProcessor) Close() error {
	k.logger.Debug("Initiated shutdown of processor goroutines")
	close(k.closed)
	k.wg.Wait()
	k.logger.Info("Completed shutdown of processor goroutines")
	return nil
}
