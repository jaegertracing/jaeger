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
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/ports"
)

const statusHTTPHostPort = "status.http.host-port"

// Command for check component status.
func Command(v *viper.Viper, adminPort int) *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "Print the status.",
		Long:  `Print Jaeger component status information, exit non-zero on any error.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			url := convert(v.GetString(statusHTTPHostPort))
			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)
			fmt.Println(string(body))
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("abnormal value of http status code: %v", resp.StatusCode)
			}
			return nil
		},
	}
	c.Flags().AddGoFlagSet(flags(&flag.FlagSet{}, adminPort))
	v.BindPFlags(c.Flags())
	return c
}

func flags(flagSet *flag.FlagSet, adminPort int) *flag.FlagSet {
	adminPortStr := ports.PortToHostPort(adminPort)
	flagSet.String(statusHTTPHostPort, adminPortStr, fmt.Sprintf(
		"The host:port (e.g. 127.0.0.1%s or %s) for the health check", adminPortStr, adminPortStr))
	return flagSet
}

func convert(httpHostPort string) string {
	if strings.HasPrefix(httpHostPort, ":") {
		return fmt.Sprintf("http://127.0.0.1%s", httpHostPort)
	}
	return fmt.Sprintf("http://%s", httpHostPort)
}
