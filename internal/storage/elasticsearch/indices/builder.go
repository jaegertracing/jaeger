// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"time"

	"go.uber.org/zap"
)

// RotationBuilderOptions holds the parameters needed to construct a Rotation.
type RotationBuilderOptions struct {
	// IndexPrefix is the fully resolved prefix (e.g., "jaeger-span-" or "prod-jaeger-span-").
	IndexPrefix string
	// DateLayout is the time format for periodic indices (e.g., "2006-01-02").
	DateLayout string
	// RolloverFrequency is the negative duration for stepping through time ranges.
	RolloverFrequency time.Duration
	// UseReadWriteAliases indicates alias-based rotation.
	UseReadWriteAliases bool
	// WriteSuffix is appended to IndexPrefix for the write alias (e.g., "write" or "archive").
	WriteSuffix string
	// ReadSuffix is appended to IndexPrefix for the read alias (e.g., "read" or "archive").
	ReadSuffix string
	// ExplicitWriteAlias overrides all other write target logic when non-empty.
	ExplicitWriteAlias string
	// ExplicitReadAlias overrides all other read target logic when non-empty.
	ExplicitReadAlias string
	// RemoteReadClusters is the list of remote cluster names for cross-cluster reads.
	RemoteReadClusters []string
	// Logger for debug logging of read targets.
	Logger *zap.Logger
}

// BuildRotation constructs the appropriate Rotation from the given options.
func BuildRotation(opts RotationBuilderOptions) Rotation {
	var r Rotation

	switch {
	case opts.ExplicitWriteAlias != "" && opts.ExplicitReadAlias != "":
		r = NewAliasRotation(opts.ExplicitWriteAlias, opts.ExplicitReadAlias)
	case opts.UseReadWriteAliases:
		writeAlias := opts.IndexPrefix + opts.WriteSuffix
		readAlias := opts.IndexPrefix + opts.ReadSuffix
		r = NewAliasRotation(writeAlias, readAlias)
	default:
		r = NewPeriodicRotation(opts.IndexPrefix, opts.DateLayout, opts.RolloverFrequency)
	}

	if len(opts.RemoteReadClusters) > 0 {
		r = NewRemoteClusterRotation(r, opts.RemoteReadClusters)
	}

	if opts.Logger != nil {
		r = NewLoggingRotation(r, opts.Logger)
	}

	return r
}
