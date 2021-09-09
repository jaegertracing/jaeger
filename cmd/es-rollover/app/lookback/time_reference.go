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

package lookback

import "time"

func getTimeReference(currentTime time.Time, units string, unitCount int) time.Time {
	switch units {
	case "minutes":
		return currentTime.Truncate(time.Minute).Add(-time.Duration(unitCount) * time.Minute)
	case "hours":
		return currentTime.Truncate(time.Hour).Add(-time.Duration(unitCount) * time.Hour)
	case "days":
		year, month, day := currentTime.Date()
		tomorrowMidnight := time.Date(year, month, day, 0, 0, 0, 0, currentTime.Location()).AddDate(0, 0, 1)
		return tomorrowMidnight.Add(-time.Hour * 24 * time.Duration(unitCount))
	case "weeks":
		year, month, day := currentTime.Date()
		tomorrowMidnight := time.Date(year, month, day, 0, 0, 0, 0, currentTime.Location()).AddDate(0, 0, 1)
		return tomorrowMidnight.Add(-time.Hour * 24 * time.Duration(7*unitCount))
	case "months":
		year, month, day := currentTime.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, currentTime.Location()).AddDate(0, -1*unitCount, 0)
	case "years":
		year, month, day := currentTime.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, currentTime.Location()).AddDate(-1*unitCount, 0, 0)
	}
	return currentTime.Truncate(time.Second).Add(-time.Duration(unitCount) * time.Second)
}
