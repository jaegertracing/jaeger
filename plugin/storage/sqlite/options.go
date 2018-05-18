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

package sqlite

import (
	"flag"

	"github.com/spf13/viper"
)

const defaultSqliteFilePath = "./jaeger.db"

// Options contains various type of sqlite configuration
type Options struct {
	file string //the sqlite db file
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet)
}

func addFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		"file",
		defaultSqliteFilePath,
		"The sqlite file path is required by Sqlite")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.file = v.GetString("file")

}

// NewOptions creates a new Options struct.
func NewOptions() *Options {
	options := &Options{
		file: defaultSqliteFilePath,
	}
	return options
}
