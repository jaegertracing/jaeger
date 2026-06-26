// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import "time"

// AliasedRotation writes to a write alias and reads from a read alias.
// Used when rollover (manual or ILM/ISM) manages index rotation externally.
type AliasedRotation struct {
	writeAlias string
	readAlias  string
}

var _ Rotation = (*AliasedRotation)(nil)

func NewAliasedRotation(writeAlias, readAlias string) *AliasedRotation {
	return &AliasedRotation{
		writeAlias: writeAlias,
		readAlias:  readAlias,
	}
}

func (s *AliasedRotation) WriteTarget(time.Time) string {
	return s.writeAlias
}

func (s *AliasedRotation) ReadTargets(time.Time, time.Time) []string {
	return []string{s.readAlias}
}

func (*AliasedRotation) WriteOpType() WriteOpType { return WriteOpIndex }
