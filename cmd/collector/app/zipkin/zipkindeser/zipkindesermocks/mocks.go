// Copyright (c) 2023 The Jaeger Authors.
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

package zipkindesermocks

import "fmt"

var (
	endpointFmt = `{"serviceName": "%s", "ipv4": "%s", "ipv6": "%s", "port": %d}`
	annoFmt     = `{"value": "%s", "timestamp": %d, "endpoint": %s}`
	binaAnnoFmt = `{"key": "%s", "value": "%s", "endpoint": %s}`
	spanFmt     = `[{"name": "%s", "id": "%s", "parentId": "%s", "traceId": "%s", "timestamp": %d, "duration": %d, "debug": %t, "annotations": [%s], "binaryAnnotations": [%s]}]`
)

func CreateEndpoint(serviveName string, ipv4 string, ipv6 string, port int) string {
	return fmt.Sprintf(endpointFmt, serviveName, ipv4, ipv6, port)
}

func CreateAnno(val string, ts int, endpoint string) string {
	return fmt.Sprintf(annoFmt, val, ts, endpoint)
}

func CreateBinAnno(key string, val string, endpoint string) string {
	return fmt.Sprintf(binaAnnoFmt, key, val, endpoint)
}

func CreateSpan(name string, id string, parentID string, traceID string, ts int64, duration int64, debug bool,
	anno string, binAnno string,
) string {
	return fmt.Sprintf(spanFmt, name, id, parentID, traceID, ts, duration, debug, anno, binAnno)
}
