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

package hostname

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestAsIdentifier(t *testing.T) {
	var hostname1 string
	var hostname2 string

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		var err error
		hostname1, err = AsIdentifier()
		require.NoError(t, err)
		wg.Done()
	}()
	go func() {
		var err error
		hostname2, err = AsIdentifier()
		require.NoError(t, err)
		wg.Done()
	}()
	wg.Wait()

	actualHostname, _ := os.Hostname()
	assert.Equal(t, hostname1, hostname2)
	assert.True(t, strings.HasPrefix(hostname1, actualHostname))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
