// Copyright (c) 2021 The Jaeger Authors.
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

package fswatcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFsWatcher(t *testing.T) {
	w, err := NewWatcher()
	require.NoError(t, err)
	assert.IsType(t, &fsnotifyWatcherWrapper{}, w)

	err = w.Add("foo")
	assert.Error(t, err)

	err = w.Add("../../cmd/query/app/fixture/ui-config.json")
	assert.NoError(t, err)

	events := w.Events()
	assert.NotZero(t, events)

	errs := w.Errors()
	assert.NotZero(t, errs)
}
