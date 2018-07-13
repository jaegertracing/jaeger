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

package badger

import (
	"fmt"
	"os"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestInitializationErrors(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	dir := "/root/this_should_fail" // If this test fails, you have some issues in your system
	keyParam := fmt.Sprintf("--badger.directory-key=%s", dir)
	valueParam := fmt.Sprintf("--badger.directory-value=%s", dir)

	command.ParseFlags([]string{
		"--badger.ephemeral=false",
		"--badger.consistency=true",
		keyParam,
		valueParam,
	})
	f.InitFromViper(v)

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	assert.Error(t, err)
}

func TestForCodecov(t *testing.T) {
	// These tests are testing our vendor packages and are intended to satisfy Codecov.
	f := NewFactory()
	v, _ := config.Viperize(f.AddFlags)
	f.InitFromViper(v)

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	assert.NoError(t, err)

	// Now, remove the badger directories
	err = os.RemoveAll(f.tmpDir)
	assert.NoError(t, err)

	// Now try to close, since the files have been deleted this should throw an error hopefully
	err = f.Close()
	assert.Error(t, err)
}

func TestMaintenanceRun(t *testing.T) {
	// For Codecov - this does not test anything
	f := NewFactory()
	v, _ := config.Viperize(f.AddFlags)
	f.InitFromViper(v)
	// Safeguard
	assert.True(t, LastMaintenanceRun.Value() == 0)
	// Lets speed up the maintenance ticker..
	f.maintenanceInterval = time.Duration(10) * time.Millisecond
	f.Initialize(metrics.NullFactory, zap.NewNop())

	sleeps := 0
Wait:
	for LastMaintenanceRun.Value() == 0 && sleeps < 8 {
		if LastMaintenanceRun.Value() > 0 {
			break Wait
		}
		time.Sleep(time.Duration(50) * time.Millisecond)
		sleeps++
	}
	err := f.Close()
	assert.NoError(t, err)
	assert.True(t, LastMaintenanceRun.Value() > 0)
}
