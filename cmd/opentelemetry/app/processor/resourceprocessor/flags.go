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

package resourceprocessor

import (
	"flag"
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
)

// AddFlags adds flags for Options.
func AddFlags(flags *flag.FlagSet) {
	flags.String(reporter.AgentTagsDeprecated, "", fmt.Sprintf("(deprecated, use --%s) One or more tags to be added to the Process tags of all spans passing through this agent. Ex: key1=value1,key2=${envVar:defaultValue}", resourceLabels))
	flags.String(resourceLabels, "", "One or more tags to be added to the Process tags of all spans passing through this agent. Ex: key1=value1,key2=${envVar:defaultValue}")
}
