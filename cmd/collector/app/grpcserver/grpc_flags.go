package grpcserver

import (
	"flag"
	"github.com/spf13/viper"
)

const (
	grpcTLSCert = "grpc.tls.cert"
	grpcTLSKey  = "grpc.tls.key"
)

// GrpcOptions holds configuration for grpc server
type GrpcOptions struct {
	// TLSCertPath is the path of the TLS certificate
	TLSCertPath string
	// TLSKeyPath is the path of the TLS Key
	TLSKeyPath string
}

// AddFlags adds flags for CollectorOptions
func AddFlags(flags *flag.FlagSet) {
	flags.String(grpcTLSCert, "", "Path to the grpc TLS certificate")
	flags.String(grpcTLSKey, "", "Path to the grpc TLS key")
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *GrpcOptions) InitFromViper(v *viper.Viper) *GrpcOptions {
	cOpts.TLSCertPath = v.GetString(grpcTLSCert)
	cOpts.TLSKeyPath = v.GetString(grpcTLSKey)
	return cOpts
}
