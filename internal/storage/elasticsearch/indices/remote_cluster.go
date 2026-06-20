// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import "time"

// RemoteClusterRotation wraps a Rotation to add cross-cluster read targets.
type RemoteClusterRotation struct {
	inner          Rotation
	remoteClusters []string
}

var _ Rotation = (*RemoteClusterRotation)(nil)

func NewRemoteClusterRotation(inner Rotation, remoteClusters []string) *RemoteClusterRotation {
	return &RemoteClusterRotation{
		inner:          inner,
		remoteClusters: remoteClusters,
	}
}

func (r *RemoteClusterRotation) WriteTarget(spanTime time.Time) string {
	return r.inner.WriteTarget(spanTime)
}

func (r *RemoteClusterRotation) ReadTargets(startTime, endTime time.Time) []string {
	local := r.inner.ReadTargets(startTime, endTime)
	if len(r.remoteClusters) == 0 {
		return local
	}
	result := make([]string, 0, len(local)*(1+len(r.remoteClusters)))
	for _, idx := range local {
		result = append(result, idx)
		for _, cluster := range r.remoteClusters {
			result = append(result, cluster+":"+idx)
		}
	}
	return result
}

func (r *RemoteClusterRotation) WriteOpType() string { return r.inner.WriteOpType() }
