package grpcutil

import (
	"net/http"
	"strings"

	"google.golang.org/grpc"
)

// CombineHandlers returns an http.Handler that delegates to grpcServer on incoming gRPC
// connections or otherHandler otherwise.
func CombineHandlers(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		println("http version", r.ProtoMajor, "Content-Type", r.Header.Get("Content-Type"))
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			println("service grpc")
			grpcServer.ServeHTTP(w, r)
		} else {
			println("service http")
			otherHandler.ServeHTTP(w, r)
		}
	})
}
