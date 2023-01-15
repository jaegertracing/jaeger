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

import "testing"

// Fake test to provide test coverage.
func TestMocks(m *testing.T) {
	CreateEndpoint("serviceName", "ipv4", "ipv6", 1)
	CreateAnno("val string", 100, "endpoint")
	CreateBinAnno("key", "val", "endpoint")
	CreateSpan("name", "id", "parentID", "traceID", 100, 100, false, "anno", "binAnno")
}
