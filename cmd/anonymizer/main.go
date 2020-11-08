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

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	app "github.com/jaegertracing/jaeger/cmd/anonymizer/app"
	writer "github.com/jaegertracing/jaeger/cmd/anonymizer/app/writer"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var logger, _ = zap.NewDevelopment()

type grpcClient struct {
	api_v2.QueryServiceClient
	conn *grpc.ClientConn
}

func newGRPCClient(addr string) *grpcClient {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	if err != nil {
		logger.Fatal("failed to connect with the jaeger-query service", zap.Error(err))
	}

	return &grpcClient{
		QueryServiceClient: api_v2.NewQueryServiceClient(conn),
		conn:               conn,
	}
}

func main() {
	var command = &cobra.Command{
		Use:   "jaeger-anonymizer",
		Short: "Jaeger anonymizer hashes fields of a trace for easy sharing",
		Long:  `Jaeger anonymizer queries Jaeger query for a trace, anonymizes fields, and store in file`,
		Run: func(cmd *cobra.Command, args []string) {
			prefix := app.OutputDir + "/" + app.TraceID
			conf := writer.Config{
				MaxSpansCount:  app.MaxSpansCount,
				CapturedFile:   prefix + ".orig",
				AnonymizedFile: prefix + ".anon",
				MappingFile:    prefix + ".map",
			}

			writer, err := writer.New(conf, logger, app.HashStandardTags, app.HashCustomTags, app.HashLogs, app.HashProcess)
			if err != nil {
				logger.Error("error while creating writer object", zap.Error(err))
			}

			queryEndpoint := app.QueryGRPCHost + ":" + strconv.Itoa(app.QueryGRPCPort)
			logger.Info(queryEndpoint)

			client := newGRPCClient(queryEndpoint)
			defer client.conn.Close()

			traceID, err := model.TraceIDFromString(app.TraceID)
			if err != nil {
				logger.Fatal("failed to convert the provided trace id", zap.Error(err))
			}
			logger.Info(app.TraceID)

			response, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
				TraceID: traceID,
			})
			if err != nil {
				logger.Fatal("failed to fetch the provided trace id", zap.Error(err))
			}

			spanResponseChunk, err := response.Recv()
			if err == spanstore.ErrTraceNotFound {
				logger.Fatal("failed to find the provided trace id", zap.Error(err))
			}
			if err != nil {
				logger.Fatal("failed to fetch spans of provided trace id", zap.Error(err))
			}

			spans := spanResponseChunk.GetSpans()

			for _, span := range spans {
				writer.WriteSpan(&span)
			}
		},
	}

	app.AddFlags(command)

	command.AddCommand(version.Command())

	if error := command.Execute(); error != nil {
		fmt.Println(error.Error())
		os.Exit(1)
	}
}
