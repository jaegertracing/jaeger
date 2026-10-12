// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

type dockerStage struct {
	name string
	user string
}

// The cmd directory has no Go package of its own, so the images of all
// commands are checked here. Every stage in these Dockerfiles is a
// published image.
func TestImagesRunWithNonRootUserAndGroup(t *testing.T) {
	dockerfiles, err := filepath.Glob("../*/Dockerfile")
	require.NoError(t, err)
	require.NotEmpty(t, dockerfiles)

	for _, dockerfile := range dockerfiles {
		t.Run(filepath.Base(filepath.Dir(dockerfile)), func(t *testing.T) {
			content, err := os.ReadFile(dockerfile)
			require.NoError(t, err)
			stages := parseDockerStages(string(content))
			require.NotEmpty(t, stages)

			for _, stage := range stages {
				uid, gid, hasGroup := strings.Cut(stage.user, ":")
				assert.True(t, hasGroup, "stage %s: want USER <uid>:<gid>, got %q", stage.name, stage.user)
				assert.NotContains(t, []string{"", "0", "root"}, uid, "stage %s: user", stage.name)
				assert.NotContains(t, []string{"", "0", "root"}, gid, "stage %s: group", stage.name)
			}
		})
	}
}

// parseDockerStages returns the final USER of each build stage, with the
// stage's own ARG defaults substituted.
func parseDockerStages(dockerfile string) []dockerStage {
	var stages []dockerStage
	args := make(map[string]string)
	for _, line := range strings.Split(dockerfile, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "FROM":
			stages = append(stages, dockerStage{name: fields[len(fields)-1]})
			args = make(map[string]string)
		case "ARG":
			if name, value, ok := strings.Cut(fields[1], "="); ok {
				args[name] = value
			}
		case "USER":
			if len(stages) > 0 {
				stages[len(stages)-1].user = os.Expand(fields[1], func(name string) string { return args[name] })
			}
		default:
			// No other instruction affects the user a stage runs as.
		}
	}
	return stages
}
