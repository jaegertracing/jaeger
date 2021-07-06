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

package hostname

import (
	"crypto/rand"
	"fmt"
	"os"
)

// AsIdentifier uses the hostname of the os and postfixes a short random string to guarantee uniqueness
// The returned value is appropriate to use as a convenient unique identifier.
func AsIdentifier() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	buff := make([]byte, 8)
	_, err = rand.Read(buff)
	if err != nil {
		return "", err
	}

	return hostname + "-" + fmt.Sprintf("%2x", buff), nil
}
