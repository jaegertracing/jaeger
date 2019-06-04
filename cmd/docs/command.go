// Copyright (c) 2019 The Jaeger Authors.
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

package docs

import (
	"flag"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/viper"
)

const (
	formatFlag = "format"
	dirFlag    = "dir"
)

var (
	formats = []string{"md", "man", "rst", "yaml"}
)

// Command for generating flags/commands documentation.
// It generates the documentation for all commands starting at parent.
func Command(v *viper.Viper) *cobra.Command {
	c := &cobra.Command{
		Use:   "docs",
		Short: "Generates documentation",
		Long:  `Generates command and flags documentation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			for cmd.Parent() != nil {
				cmd = cmd.Parent()
			}
			dir := v.GetString(dirFlag)
			log.Printf("Generating documentation in %v", dir)
			switch v.GetString(formatFlag) {
			case "md":
				return doc.GenMarkdownTree(cmd, dir)
			case "man":
				return man(cmd, dir)
			case "rst":
				return doc.GenReSTTree(cmd, dir)
			case "yaml":
				return doc.GenYamlTree(cmd, dir)
			default:
				return fmt.Errorf("undefined value of %v, possible values are: %v", formatFlag, formats)
			}
		},
	}
	c.Flags().AddGoFlagSet(flags(&flag.FlagSet{}))
	v.BindPFlags(c.Flags())
	return c
}

func flags(flagSet *flag.FlagSet) *flag.FlagSet {
	flagSet.String(
		formatFlag,
		formats[0],
		fmt.Sprintf("Supported formats: %v.", formats))
	flagSet.String(
		dirFlag,
		"./",
		"Directory where generate the documentation.")
	return flagSet
}

func man(cmd *cobra.Command, dir string) error {
	header := &doc.GenManHeader{
		Title:   cmd.Use,
		Section: "1",
	}
	return doc.GenManTree(cmd, header, dir)
}
