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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsIdentifier(t *testing.T) {
	hostname1, err := AsIdentifier()
	require.NoError(t, err)
	hostname2, err := AsIdentifier()
	require.NoError(t, err)

	actualHostname, _ := os.Hostname()

	assert.NotEqual(t, hostname1, hostname2)
	assert.True(t, strings.HasPrefix(hostname1, actualHostname))
	assert.True(t, strings.HasPrefix(hostname2, actualHostname))
}
