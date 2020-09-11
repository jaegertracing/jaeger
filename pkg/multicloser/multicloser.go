// Copyright (c) 2020 The Jaeger Authors.
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

package multicloser

import (
	"io"

	"github.com/jaegertracing/jaeger/pkg/multierror"
)

// MultiCloser wraps multiple io.Closer interfaces
type MultiCloser struct {
	closers []io.Closer
}

var _ io.Closer = (*MultiCloser)(nil)

// Close implements io.Closer
func (m MultiCloser) Close() error {
	var errs []error
	for _, c := range m.closers {
		if c == nil {
			continue
		}
		err := c.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return multierror.Wrap(errs)
}

// Wrap creates io.Closer that encapsulates multiple io.Closer interfaces
func Wrap(closers ...io.Closer) *MultiCloser {
	return &MultiCloser{
		closers: closers,
	}
}
