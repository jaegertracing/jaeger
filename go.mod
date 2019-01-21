module github.com/jaegertracing/jaeger

go 1.12

require (
	github.com/DataDog/zstd v1.3.4 // indirect
	github.com/Shopify/sarama v1.20.0
	github.com/VividCortex/gohistogram v1.0.0 // indirect
	github.com/apache/thrift v0.0.0-20151001171628-53dd39833a08
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973 // indirect
	github.com/bsm/sarama-cluster v2.1.13+incompatible
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/crossdock/crossdock-go v0.0.0-20160816171116-049aabb0122b
	github.com/eapache/go-resiliency v1.1.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8 // indirect
	github.com/go-kit/kit v0.5.0 // indirect
	github.com/go-openapi/analysis v0.17.2 // indirect
	github.com/go-openapi/errors v0.17.2
	github.com/go-openapi/jsonpointer v0.17.2 // indirect
	github.com/go-openapi/jsonreference v0.17.2 // indirect
	github.com/go-openapi/loads v0.17.0
	github.com/go-openapi/runtime v0.17.2
	github.com/go-openapi/spec v0.17.2
	github.com/go-openapi/strfmt v0.17.2
	github.com/go-openapi/swag v0.17.2
	github.com/go-openapi/validate v0.17.2
	github.com/gocql/gocql v0.0.0-20181124151448-70385f88b28b
	github.com/gogo/googleapis v1.1.0
	github.com/gogo/protobuf v1.2.0
	github.com/golang/protobuf v1.2.0
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/handlers v0.0.0-20161206055144-3a5767ca75ec
	github.com/gorilla/mux v1.3.0
	github.com/grpc-ecosystem/grpc-gateway v0.0.0-20180312001938-58f78b988bc3
	github.com/kr/pretty v0.1.0
	github.com/ledor473/jaeger-storage-badger v0.0.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/opentracing-contrib/go-stdlib v0.0.0-20181101210145-c9628a4f0148
	github.com/opentracing/opentracing-go v1.0.2
	github.com/pierrec/lz4 v0.0.0-20181005164709-635575b42742 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.8.0
	github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910 // indirect
	github.com/prometheus/common v0.0.0-20181126121408-4724e9255275 // indirect
	github.com/prometheus/procfs v0.0.0-20181204211112-1dc9a6cbc91a // indirect
	github.com/rakyll/statik v0.1.5
	github.com/rcrowley/go-metrics v0.0.0-20181016184325-3113b8401b8a // indirect
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.3.1
	github.com/stretchr/objx v0.1.1 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/uber-go/atomic v1.3.2 // indirect
	github.com/uber/jaeger-client-go v0.0.0-20190116124224-6733ee486c78
	github.com/uber/jaeger-lib v2.0.0+incompatible
	github.com/uber/tchannel-go v1.1.0
	go.uber.org/atomic v1.3.2
	go.uber.org/zap v1.9.1
	golang.org/x/net v0.0.0-20190119204137-ed066c81e75e
	google.golang.org/genproto v0.0.0-20181202183823-bd91e49a0898 // indirect
	google.golang.org/grpc v1.16.0
	gopkg.in/olivere/elastic.v5 v5.0.53
	gopkg.in/yaml.v2 v2.2.2
)

replace github.com/ledor473/jaeger-storage-badger => ../../ledor473/jaeger-storage-badger
