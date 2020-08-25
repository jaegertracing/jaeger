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

package status

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Command for running status check against the AdminPort.
func Command(v *viper.Viper) *cobra.Command {
	adminURL := ""
	c := &cobra.Command{
		Use:   "status",
		Short: "Print the status.",
		Long:  `Prints admin status information, exit non-zero on any error.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			selfAdminFlag := cmd.Flags().Lookup("admin.http.host-port")
			for cmd.Parent() != nil {
				cmd = cmd.Parent()
			}
			parentAdminFlag := cmd.Flags().Lookup("admin.http.host-port")
			// Test if an override is set, if so, use it. Otherwise fall back to the default for this subcommand or error.
			// FIXME: cmd.flags should probably export this instead
			hostPortFlag := ""
			if selfAdminFlag.Changed {
				hostPortFlag = selfAdminFlag.Value.String()
			} else if parentAdminFlag.DefValue != ":0" {
				hostPortFlag = parentAdminFlag.Value.String()
			} else {
				return fmt.Errorf("no default admin port available for %s", cmd.Name())
			}
			// Parse the selected flag value
			var err error
			adminURL, err = parseAdminHostPort(hostPortFlag)
			if err != nil {
				return err
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := http.Get(adminURL)
			if err != nil {
				log.Printf("error: %v", err)
				os.Exit(1)
			}
			log.Printf("ok: %v", resp)
			os.Exit(0)
		},
	}
	// Add support for overriding the default http server flags
	f := &flag.FlagSet{}
	adminServer := flags.NewAdminServer(":0")
	adminServer.AddFlags(f)
	c.Flags().AddGoFlagSet(f)
	v.BindPFlags(c.Flags())
	return c
}

// FIXME: cmd.flags should probably export this instead
func parseAdminHostPort(adminHostPort string) (string, error) {
	adminListeningAddr := strings.SplitN(adminHostPort, ":", 2)
	adminHost := adminListeningAddr[0]
	if adminHost == "" || adminHost == "0.0.0.0" {
		adminHost = "localhost"
	}
	adminPort, err := strconv.ParseInt(adminListeningAddr[1], 10, 16) // 2**16 or 65_535 is the maximum listening port
	if err != nil {
		return "", fmt.Errorf("invalid port `%s`: %w", adminHostPort, err)
	}
	if adminPort == 0 {
		return "", fmt.Errorf("invalid port `%s`", adminHostPort)
	}
	return fmt.Sprintf("http://%s:%d/", adminHost, adminPort), nil
}
