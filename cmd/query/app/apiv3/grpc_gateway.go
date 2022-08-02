// Copyright (c) 2021 The Jaeger Authors.
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

package apiv3

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
)

// RegisterGRPCGateway registers api_v3 endpoints into provided mux.
func RegisterGRPCGateway(ctx context.Context, logger *zap.Logger, r *mux.Router, basePath string, grpcEndpoint string, grpcTLS tlscfg.Options, tm *tenancy.Manager) error {
	jsonpb := &runtime.JSONPb{}

	muxOpts := []runtime.ServeMuxOption{
		runtime.WithMarshalerOption(runtime.MIMEWildcard, jsonpb),
	}
	if tm.Enabled {
		muxOpts = append(muxOpts, runtime.WithMetadata(tm.MetadataAnnotator()))
	}

	grpcGatewayMux := runtime.NewServeMux(muxOpts...)
	var handler http.Handler = grpcGatewayMux
	if basePath != "/" {
		handler = http.StripPrefix(basePath, grpcGatewayMux)
	}
	r.PathPrefix("/api/v3/").Handler(handler)

	var dialOpts []grpc.DialOption
	if grpcTLS.Enabled {
		tlsCfg, err := grpcTLS.Config(logger)
		if err != nil {
			return err
		}
		creds := credentials.NewTLS(tlsCfg)
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return api_v3.RegisterQueryServiceHandlerFromEndpoint(ctx, grpcGatewayMux, grpcEndpoint, dialOpts)
}
