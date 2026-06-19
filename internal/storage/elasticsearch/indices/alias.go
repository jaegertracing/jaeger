// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import "time"

// AliasRotation writes to a write alias and reads from a read alias.
// Used when rollover (manual or ILM/ISM) manages index rotation externally.
type AliasRotation struct {
	writeAlias string
	readAlias  string
}

var _ Rotation = (*AliasRotation)(nil)

func NewAliasRotation(writeAlias, readAlias string) *AliasRotation {
	return &AliasRotation{
		writeAlias: writeAlias,
		readAlias:  readAlias,
	}
}

func (s *AliasRotation) WriteTarget(time.Time) string {
	return s.writeAlias
}

func (s *AliasRotation) ReadTargets(time.Time, time.Time) []string {
	return []string{s.readAlias}
}

func (*AliasRotation) OpType() string { return "index" }

func (*AliasRotation) UseTimeRangeFilter() bool { return true }
