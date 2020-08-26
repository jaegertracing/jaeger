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

package status

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Command for running status check against the AdminPort.
func Command(v *viper.Viper, adminPort int) *cobra.Command {
	adminServer := flags.NewAdminServer(ports.PortToHostPort(adminPort))
	adminURL := ""
	c := &cobra.Command{
		Use:   "status",
		Short: "Print the status.",
		Long:  `Prints admin status information, exit non-zero on any error.`,
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			rootCmd := cmd
			for rootCmd.Parent() != nil {
				rootCmd = rootCmd.Parent()
			}
			// FIXME: arg overrides aren't working, adminServer doesn't seem to see the current flagSet. Why?
			adminURL, err = adminServer.URL()
			if err != nil {
				return fmt.Errorf("no default admin port available for %s: %w", cmd.Name(), err)
			}
			return nil
		},
		// FIXME: Normally I'd want to return an Error and let the exit handler do the os.Exit.
		// But it's triggering --help output everytime and I don't know why.
		Run: func(cmd *cobra.Command, args []string) {
			adminURL, _ = adminServer.URL()
			resp, err := http.Get(adminURL)
			if err != nil {
				log.Printf("error: %v", err)
				os.Exit(2)
			}
			log.Printf("ok: %v", resp)
			os.Exit(0)
		},
	}
	// Add support for overriding the default http server flags
	f := &flag.FlagSet{}
	adminServer.AddFlags(f)
	c.Flags().AddGoFlagSet(f)
	v.BindPFlags(c.Flags())
	return c
}
