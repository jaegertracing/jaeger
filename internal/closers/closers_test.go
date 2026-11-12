// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package closers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

type recordingCloser struct {
	log  *[]string
	name string
	err  error
}

func (c recordingCloser) Close() error {
	*c.log = append(*c.log, c.name)
	return c.err
}

func TestClosersCloseAllAndJoinErrors(t *testing.T) {
	var log []string
	cs := Closers{
		recordingCloser{log: &log, name: "first", err: errors.New("first failed")},
		recordingCloser{log: &log, name: "second"},
		recordingCloser{log: &log, name: "third", err: errors.New("third failed")},
	}
	err := cs.Close()
	require.Equal(t, []string{"first", "second", "third"}, log)
	require.ErrorContains(t, err, "first failed")
	require.ErrorContains(t, err, "third failed")
}

func TestClosersEmpty(t *testing.T) {
	require.NoError(t, Closers(nil).Close())
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
