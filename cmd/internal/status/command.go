// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			url := convert(v.GetString(statusHTTPHostPort))
			ctx, cx := context.WithTimeout(context.Background(), time.Second)
			defer cx()
			req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
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
		return "http://127.0.0.1" + httpHostPort
	}
	return "http://" + httpHostPort
}
