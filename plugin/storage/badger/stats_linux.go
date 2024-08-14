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
	"golang.org/x/sys/unix"
)

func (f *Factory) diskStatisticsUpdate() error {
	// These stats are not interesting with Windows as there's no separate tmpfs
	// In case of ephemeral these are the same, but we'll report them separately for consistency
	var keyDirStatfs unix.Statfs_t
	// Error ignored to satisfy Codecov
	_ = unix.Statfs(f.Options.GetPrimary().KeyDirectory, &keyDirStatfs)

	var valDirStatfs unix.Statfs_t
	// Error ignored to satisfy Codecov
	_ = unix.Statfs(f.Options.GetPrimary().ValueDirectory, &valDirStatfs)

	// Using Bavail instead of Bfree to get non-priviledged user space available
	f.metrics.ValueLogSpaceAvailable.Update(int64(valDirStatfs.Bavail) * int64(valDirStatfs.Bsize))
	f.metrics.KeyLogSpaceAvailable.Update(int64(keyDirStatfs.Bavail) * int64(keyDirStatfs.Bsize))

	/*
	 TODO If we wanted to clean up oldest data to free up diskspace, we need at a minimum an index to the StartTime
	 Additionally to that, the deletion might not save anything if the ratio of removed values is lower than the RunValueLogGC's deletion ratio
	 and with the keys the LSM compaction must remove the offending files also. Thus, there's no guarantee the clean up would
	 actually reduce the amount of diskspace used any faster than allowing TTL to remove them.

	 If badger supports TimeWindow based compaction, then this should be resolved. Not available in 1.5.3
	*/
	return nil
}
