// Copyright (c) 2020 The Jaeger Authors.
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

package app

import (
	"flag"
	"io/ioutil"
	"os"

	"github.com/open-telemetry/opentelemetry-collector/service/builder"
)

// GetOTELConfigFile returns name of OTEL config file.
func GetOTELConfigFile() string {
	f := &flag.FlagSet{}
	f.SetOutput(ioutil.Discard)
	builder.Flags(f)
	// parse flags to bind the value
	f.Parse(os.Args[1:])
	return builder.GetConfigFile()
}
