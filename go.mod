module github.com/jaegertracing/jaeger

go 1.23.7

toolchain go1.24.0

require (
	github.com/ClickHouse/ch-go v0.65.1
	github.com/ClickHouse/clickhouse-go/v2 v2.32.2
	github.com/HdrHistogram/hdrhistogram-go v1.1.2
	github.com/Shopify/sarama v1.37.2
	github.com/apache/thrift v0.21.0
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2
	github.com/bsm/sarama-cluster v2.1.13+incompatible
	github.com/crossdock/crossdock-go v0.0.0-20160816171116-049aabb0122b
	github.com/dgraph-io/badger/v4 v4.5.1
	github.com/elastic/go-elasticsearch/v8 v8.17.1
	github.com/fsnotify/fsnotify v1.8.0
	github.com/go-logr/zapr v1.3.0
	github.com/gocql/gocql v1.7.0
	github.com/gogo/protobuf v1.3.2
	github.com/gorilla/handlers v1.5.2
	github.com/gorilla/mux v1.8.1
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.0
	github.com/jaegertracing/jaeger-idl v0.5.0
	github.com/kr/pretty v0.3.1
	github.com/olivere/elastic v6.2.37+incompatible
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusexporter v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckv2extension v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver v0.120.1
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver v0.120.1
	github.com/prometheus/client_golang v1.21.0
	github.com/prometheus/client_model v0.6.1
	github.com/prometheus/common v0.62.0
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6
	github.com/spf13/viper v1.19.0
	github.com/stretchr/testify v1.10.0
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	github.com/xdg-go/scram v1.1.2
	go.opentelemetry.io/collector/component v0.120.0
	go.opentelemetry.io/collector/component/componentstatus v0.120.0
	go.opentelemetry.io/collector/component/componenttest v0.120.0
	go.opentelemetry.io/collector/config/configauth v0.120.0
	go.opentelemetry.io/collector/config/configgrpc v0.120.0
	go.opentelemetry.io/collector/config/confighttp v0.120.0
	go.opentelemetry.io/collector/config/confighttp/xconfighttp v0.120.0
	go.opentelemetry.io/collector/config/configretry v1.26.0
	go.opentelemetry.io/collector/config/configtls v1.26.0
	go.opentelemetry.io/collector/confmap v1.26.0
	go.opentelemetry.io/collector/confmap/provider/envprovider v1.26.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v1.26.0
	go.opentelemetry.io/collector/confmap/provider/httpprovider v1.26.0
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v1.26.0
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.26.0
	go.opentelemetry.io/collector/connector v0.120.0
	go.opentelemetry.io/collector/connector/forwardconnector v0.120.0
	go.opentelemetry.io/collector/consumer v1.26.0
	go.opentelemetry.io/collector/consumer/consumertest v0.120.0
	go.opentelemetry.io/collector/exporter v0.120.0
	go.opentelemetry.io/collector/exporter/exportertest v0.120.0
	go.opentelemetry.io/collector/exporter/nopexporter v0.120.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.120.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.120.0
	go.opentelemetry.io/collector/extension v0.120.0
	go.opentelemetry.io/collector/extension/zpagesextension v0.120.0
	go.opentelemetry.io/collector/featuregate v1.26.0
	go.opentelemetry.io/collector/otelcol v0.120.0
	go.opentelemetry.io/collector/pdata v1.26.0
	go.opentelemetry.io/collector/pipeline v0.120.0
	go.opentelemetry.io/collector/processor v0.120.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.120.0
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.120.0
	go.opentelemetry.io/collector/processor/processortest v0.120.0
	go.opentelemetry.io/collector/receiver v0.120.0
	go.opentelemetry.io/collector/receiver/nopreceiver v0.120.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.120.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.59.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.59.0
	go.opentelemetry.io/contrib/samplers/jaegerremote v0.28.0
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.34.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.34.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.34.0
	go.opentelemetry.io/otel/exporters/prometheus v0.56.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.34.0
	go.opentelemetry.io/otel/metric v1.34.0
	go.opentelemetry.io/otel/sdk v1.34.0
	go.opentelemetry.io/otel/sdk/metric v1.34.0
	go.opentelemetry.io/otel/trace v1.34.0
	go.uber.org/automaxprocs v1.6.0
	go.uber.org/goleak v1.3.0
	go.uber.org/zap v1.27.0
	golang.org/x/net v0.35.0
	golang.org/x/sys v0.30.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.0 // indirect
	github.com/dmarkham/enumer v1.5.10 // indirect
	github.com/go-faster/city v1.0.1 // indirect
	github.com/go-faster/errors v0.7.1 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/pascaldekloe/name v1.0.1 // indirect
	github.com/paulmach/orb v0.11.1 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	go.opentelemetry.io/collector/extension/extensiontest v0.120.0 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
)

require (
	github.com/IBM/sarama v1.45.0 // indirect
	github.com/alecthomas/participle/v2 v2.1.1 // indirect
	github.com/antchfx/xmlquery v1.4.3 // indirect
	github.com/antchfx/xpath v1.3.3 // indirect
	github.com/aws/aws-msk-iam-sasl-signer-go v1.0.1 // indirect
	github.com/aws/aws-sdk-go v1.55.6 // indirect
	github.com/aws/aws-sdk-go-v2 v1.32.4 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.28.2 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.43 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.32.4 // indirect
	github.com/aws/smithy-go v1.22.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgraph-io/ristretto/v2 v2.1.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/eapache/go-resiliency v1.7.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/elastic/elastic-transport-go/v8 v8.6.1 // indirect
	github.com/elastic/go-grok v0.3.1 // indirect
	github.com/elastic/lunes v0.1.0 // indirect
	github.com/expr-lang/expr v1.16.9 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v24.12.23+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.25.1 // indirect
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mostynb/go-grpc-compression v1.2.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/kafka v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/pdatautil v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/batchpersignal v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/core/xidutils v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/kafka/topic v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/status v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/azure v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.120.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin v0.120.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/relvacode/iso8601 v1.6.0 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/shirou/gopsutil/v4 v4.25.1 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/ua-parser/uap-go v0.0.0-20240611065828-3a4781585db6 // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector v0.120.0 // indirect
	go.opentelemetry.io/collector/client v1.26.0
	go.opentelemetry.io/collector/config/configcompression v1.26.0 // indirect
	go.opentelemetry.io/collector/config/confignet v1.26.0
	go.opentelemetry.io/collector/config/configopaque v1.26.0
	go.opentelemetry.io/collector/config/configtelemetry v0.120.0 // indirect
	go.opentelemetry.io/collector/confmap/xconfmap v0.120.0
	go.opentelemetry.io/collector/connector/connectortest v0.120.0 // indirect
	go.opentelemetry.io/collector/connector/xconnector v0.120.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.120.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.120.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.120.0 // indirect
	go.opentelemetry.io/collector/exporter/debugexporter v0.120.0
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.120.0 // indirect
	go.opentelemetry.io/collector/exporter/xexporter v0.120.0 // indirect
	go.opentelemetry.io/collector/extension/auth v0.120.0 // indirect
	go.opentelemetry.io/collector/extension/extensioncapabilities v0.120.0
	go.opentelemetry.io/collector/extension/xextension v0.120.0 // indirect
	go.opentelemetry.io/collector/internal/fanoutconsumer v0.120.0 // indirect
	go.opentelemetry.io/collector/internal/memorylimiter v0.120.0 // indirect
	go.opentelemetry.io/collector/internal/sharedcomponent v0.120.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.120.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.120.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.120.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.120.0 // indirect
	go.opentelemetry.io/collector/processor/xprocessor v0.120.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.120.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.120.0 // indirect
	go.opentelemetry.io/collector/semconv v0.120.0
	go.opentelemetry.io/collector/service v0.120.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.9.0 // indirect
	go.opentelemetry.io/contrib/config v0.14.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.34.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.59.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.10.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.34.0 // indirect
	go.opentelemetry.io/otel/log v0.10.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.10.0 // indirect
	go.opentelemetry.io/proto/otlp v1.5.0
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // indirect
	golang.org/x/text v0.22.0 // indirect
	gonum.org/v1/gonum v0.15.1 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/Shopify/sarama => github.com/Shopify/sarama v1.33.0
