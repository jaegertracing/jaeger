module github.com/jaegertracing/jaeger

go 1.16

require (
	github.com/Shopify/sarama v1.29.1
	github.com/apache/thrift v0.14.2
	github.com/bsm/sarama-cluster v2.1.13+incompatible
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/crossdock/crossdock-go v0.0.0-20160816171116-049aabb0122b
	github.com/dgraph-io/badger v1.6.2 // indirect
	github.com/dgraph-io/badger/v3 v3.2103.0
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-openapi/errors v0.20.0
	github.com/go-openapi/loads v0.20.2
	github.com/go-openapi/runtime v0.19.28
	github.com/go-openapi/spec v0.20.3
	github.com/go-openapi/strfmt v0.20.1
	github.com/go-openapi/swag v0.19.15
	github.com/go-openapi/validate v0.20.2
	github.com/gocql/gocql v0.0.0-20200228163523-cd4b606dd2fb
	github.com/gogo/googleapis v1.4.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/hashicorp/go-hclog v0.16.2
	github.com/hashicorp/go-plugin v1.4.2
	github.com/kr/pretty v0.2.1
	github.com/mjibson/esc v0.2.0
	github.com/olivere/elastic v6.2.37+incompatible
	github.com/opentracing-contrib/go-grpc v0.0.0-20191001143057-db30781987df
	github.com/opentracing-contrib/go-stdlib v1.0.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.30.0
	github.com/rs/cors v1.8.0
	github.com/securego/gosec v0.0.0-20200203094520-d13bb6d2420c
	github.com/soheilhy/cmux v0.1.5
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/uber/jaeger-client-go v2.29.1+incompatible
	github.com/uber/jaeger-lib v2.4.1+incompatible
	github.com/vektra/mockery v0.0.0-20181123154057-e78b021dcbb5
	github.com/wadey/gocovmerge v0.0.0-20160331181800-b5bfa59ec0ad
	github.com/xdg-go/scram v1.0.2
	go.opentelemetry.io/collector v0.30.1
	go.uber.org/atomic v1.9.0
	go.uber.org/automaxprocs v1.4.0
	go.uber.org/zap v1.18.1
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e
	golang.org/x/sys v0.0.0-20210615035016-665e8c7367d1
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
	gopkg.in/yaml.v2 v2.4.0
	honnef.co/go/tools v0.2.0
)
