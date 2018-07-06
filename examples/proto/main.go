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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gogo/gateway"
	// "github.com/grpc-ecosystem/go-grpc-middleware/validator"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/jaegertracing/jaeger/examples/proto/server"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

var (
	isClient    = flag.Bool("client", false, "Run in client mode")
	gRPCPort    = flag.Int("grpc-port", 10000, "The gRPC server port")
	gatewayPort = flag.Int("gateway-port", 11000, "The gRPC-Gateway server port")
)

var log grpclog.LoggerV2

func init() {
	log = grpclog.NewLoggerV2(os.Stdout, os.Stderr, os.Stderr)
	grpclog.SetLoggerV2(log)
}

// serveOpenAPI serves an OpenAPI UI on /openapi-ui/
// Adapted from https://github.com/philips/grpc-gateway-example/blob/a269bcb5931ca92be0ceae6130ac27ae89582ecc/cmd/serve.go#L63
// func serveOpenAPI(mux *http.ServeMux) error {
// 	mime.AddExtensionType(".svg", "image/svg+xml")

// 	statikFS, err := fs.New()
// 	if err != nil {
// 		return err
// 	}

// 	// Expose files in static on <host>/openapi-ui
// 	fileServer := http.FileServer(statikFS)
// 	prefix := "/openapi-ui/"
// 	mux.Handle(prefix, http.StripPrefix(prefix, fileServer))
// 	return nil
// }

// Tests:
//     $ curl http://localhost:11000/api/v2/traces/123
//     $ prototool grpc ./model/proto/api_v2.proto localhost:10000 jaeger.api_v2.QueryService/GetTrace '{"id":"123"}'
func main() {
	flag.Parse()
	if *isClient {
		runClient()
		return
	}
	addr := fmt.Sprintf("localhost:%d", *gRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln("Failed to listen:", err)
	}
	s := grpc.NewServer(
	// grpc.Creds(credentials.NewServerTLSFromCert(&insecure.Cert)),
	// grpc.UnaryInterceptor(grpc_validator.UnaryServerInterceptor()),
	// grpc.StreamInterceptor(grpc_validator.StreamServerInterceptor()),
	)
	api_v2.RegisterQueryServiceServer(s, server.New())
	api_v2.RegisterCollectorServiceServer(s, server.New())

	// Serve gRPC Server
	log.Info("Serving gRPC on https://", addr)
	go func() {
		log.Fatal(s.Serve(lis))
	}()

	// See https://github.com/grpc/grpc/blob/master/doc/naming.md
	// for gRPC naming standard information.
	dialAddr := fmt.Sprintf("passthrough://localhost/%s", addr)
	conn, err := grpc.DialContext(
		context.Background(),
		dialAddr,
		grpc.WithInsecure(),
		// grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(insecure.CertPool, "")),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalln("Failed to dial server:", err)
	}

	mux := http.NewServeMux()

	jsonpb := &gateway.JSONPb{
		EmitDefaults: false,
		Indent:       "  ",
		OrigName:     true,
	}
	gwmux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, jsonpb),
		// This is necessary to get error details properly
		// marshalled in unary requests.
		runtime.WithProtoErrorHandler(runtime.DefaultHTTPProtoErrorHandler),
	)
	err = api_v2.RegisterQueryServiceHandler(context.Background(), gwmux, conn)
	if err != nil {
		log.Fatalln("Failed to register gateway:", err)
	}

	mux.Handle("/", gwmux)
	// err = serveOpenAPI(mux)
	// if err != nil {
	// 	log.Fatalln("Failed to serve OpenAPI UI")
	// }

	gatewayAddr := fmt.Sprintf("localhost:%d", *gatewayPort)
	log.Info("Serving gRPC-Gateway on https://", gatewayAddr)
	// log.Info("Serving OpenAPI Documentation on https://", gatewayAddr, "/openapi-ui/")
	gwServer := http.Server{
		Addr: gatewayAddr,
		// TLSConfig: &tls.Config{
		// 	Certificates: []tls.Certificate{insecure.Cert},
		// },
		Handler: mux,
	}
	// log.Fatalln(gwServer.ListenAndServeTLS("", ""))
	log.Fatalln(gwServer.ListenAndServe())
}

func runClient() {
	addr := fmt.Sprintf("localhost:%d", *gRPCPort)
	// Set up a connection to the server.
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := api_v2.NewCollectorServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := c.PostSpans(ctx, &api_v2.PostSpansRequest{
		Batch: model.Batch{
			Spans: []*model.Span{
				&model.Span{
					OperationName: "fake-operation",
				},
			},
		},
	})
	if err != nil {
		log.Fatalf("could not post: %v", err)
	}
	fmt.Printf("Response: %+v\n", r.Ok)
}
