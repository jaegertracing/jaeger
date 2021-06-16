module github.com/jaegertracing/jaeger

go 1.16

require (
	github.com/HdrHistogram/hdrhistogram-go v0.9.0 // indirect
	github.com/Shopify/sarama v1.29.0
	github.com/apache/thrift v0.14.1
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/bsm/sarama-cluster v2.1.13+incompatible
	github.com/crossdock/crossdock-go v0.0.0-20160816171116-049aabb0122b
	github.com/dgraph-io/badger v1.6.2
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/fatih/color v1.9.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-kit/kit v0.10.0 // indirect
	github.com/go-openapi/analysis v0.20.1 // indirect
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
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/hashicorp/go-hclog v0.16.1
	github.com/hashicorp/go-plugin v1.4.2
	github.com/hashicorp/yamux v0.0.0-20190923154419-df201c70410d // indirect
	github.com/kr/pretty v0.2.1
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mjibson/esc v0.2.0
	github.com/oklog/run v1.1.0 // indirect
	github.com/olivere/elastic v6.2.35+incompatible
	github.com/opentracing-contrib/go-grpc v0.0.0-20191001143057-db30781987df
	github.com/opentracing-contrib/go-stdlib v0.0.0-20190519235532-cf7a6c988dc9
	github.com/opentracing/opentracing-go v1.2.0
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.29.0
	github.com/rs/cors v1.7.0
	github.com/securego/gosec v0.0.0-20200203094520-d13bb6d2420c
	github.com/soheilhy/cmux v0.1.5
	github.com/spf13/cobra v0.0.7
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.0
	github.com/stretchr/testify v1.7.0
	github.com/uber/jaeger-client-go v2.29.1+incompatible
	github.com/uber/jaeger-lib v2.4.1+incompatible
	github.com/vektra/mockery v0.0.0-20181123154057-e78b021dcbb5
	github.com/wadey/gocovmerge v0.0.0-20160331181800-b5bfa59ec0ad
	github.com/xdg-go/scram v1.0.2
	go.mongodb.org/mongo-driver v1.5.2 // indirect
	go.uber.org/atomic v1.8.0
	go.uber.org/automaxprocs v1.4.0
	go.uber.org/zap v1.17.0
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
	golang.org/x/sys v0.0.0-20210603125802-9665404d3644
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/yaml.v2 v2.4.0
	honnef.co/go/tools v0.2.0
)

// TODO: remove once the 0.14.2 or 0.15.0 is released
replace github.com/apache/thrift => github.com/jaegertracing/thrift v0.14.1-patch1
