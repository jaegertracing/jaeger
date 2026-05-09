module github.com/jaegertracing/jaeger

go 1.26.0

require (
	github.com/ClickHouse/ch-go v0.71.0
	github.com/ClickHouse/clickhouse-go/v2 v2.45.0
	github.com/apache/cassandra-gocql-driver/v2 v2.1.1
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/coder/acp-go-sdk v0.6.4-0.20260227160919-584abe6abe22
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/dgraph-io/badger/v4 v4.9.1
	github.com/elastic/go-elasticsearch/v9 v9.3.1
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-logr/zapr v1.3.0
	github.com/gogo/protobuf v1.3.2
	github.com/gorilla/handlers v1.5.2
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
	github.com/jaegertracing/jaeger-idl v0.6.0
	github.com/kr/pretty v0.3.1
	github.com/modelcontextprotocol/go-sdk v1.5.0
	github.com/olivere/elastic/v7 v7.0.32
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusexporter v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckv2extension v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/sigv4authextension v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/core/xidutils v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver v0.151.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver v0.151.0
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.67.5
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/collector/client v1.57.0
	go.opentelemetry.io/collector/component v1.57.0
	go.opentelemetry.io/collector/component/componentstatus v0.151.0
	go.opentelemetry.io/collector/component/componenttest v0.151.0
	go.opentelemetry.io/collector/config/configauth v1.57.0
	go.opentelemetry.io/collector/config/configgrpc v0.151.0
	go.opentelemetry.io/collector/config/confighttp v0.151.0
	go.opentelemetry.io/collector/config/confighttp/xconfighttp v0.151.0
	go.opentelemetry.io/collector/config/configmiddleware v1.57.0
	go.opentelemetry.io/collector/config/confignet v1.57.0
	go.opentelemetry.io/collector/config/configoptional v1.57.0
	go.opentelemetry.io/collector/config/configretry v1.57.0
	go.opentelemetry.io/collector/config/configtelemetry v0.151.0
	go.opentelemetry.io/collector/config/configtls v1.57.0
	go.opentelemetry.io/collector/confmap v1.57.0
	go.opentelemetry.io/collector/confmap/provider/envprovider v1.57.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v1.57.0
	go.opentelemetry.io/collector/confmap/provider/httpprovider v1.57.0
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v1.57.0
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.57.0
	go.opentelemetry.io/collector/confmap/xconfmap v0.151.0
	go.opentelemetry.io/collector/connector v0.151.0
	go.opentelemetry.io/collector/connector/forwardconnector v0.151.0
	go.opentelemetry.io/collector/consumer v1.57.0
	go.opentelemetry.io/collector/consumer/consumertest v0.151.0
	go.opentelemetry.io/collector/exporter v1.57.0
	go.opentelemetry.io/collector/exporter/debugexporter v0.151.0
	go.opentelemetry.io/collector/exporter/exporterhelper v0.151.0
	go.opentelemetry.io/collector/exporter/exportertest v0.151.0
	go.opentelemetry.io/collector/exporter/nopexporter v0.151.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.151.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.151.0
	go.opentelemetry.io/collector/extension v1.57.0
	go.opentelemetry.io/collector/extension/extensionauth v1.57.0
	go.opentelemetry.io/collector/extension/extensioncapabilities v0.151.0
	go.opentelemetry.io/collector/extension/zpagesextension v0.151.0
	go.opentelemetry.io/collector/featuregate v1.57.0
	go.opentelemetry.io/collector/otelcol v0.151.0
	go.opentelemetry.io/collector/pdata v1.57.0
	go.opentelemetry.io/collector/pdata/xpdata v0.151.0
	go.opentelemetry.io/collector/pipeline v1.57.0
	go.opentelemetry.io/collector/processor v1.57.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.151.0
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.151.0
	go.opentelemetry.io/collector/processor/processorhelper v0.151.0
	go.opentelemetry.io/collector/processor/processortest v0.151.0
	go.opentelemetry.io/collector/receiver v1.57.0
	go.opentelemetry.io/collector/receiver/nopreceiver v0.151.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.151.0
	go.opentelemetry.io/collector/service v0.151.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.68.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0
	go.opentelemetry.io/contrib/samplers/jaegerremote v0.37.0
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.43.0
	go.opentelemetry.io/otel/exporters/prometheus v0.65.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.43.0
	go.opentelemetry.io/otel/metric v1.43.0
	go.opentelemetry.io/otel/sdk v1.43.0
	go.opentelemetry.io/otel/sdk/metric v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
	go.uber.org/automaxprocs v1.6.0
	go.uber.org/goleak v1.3.0
	go.uber.org/zap v1.28.0
	go.yaml.in/yaml/v3 v3.0.4
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f
	golang.org/x/sys v0.43.0
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require (
	cloud.google.com/go/auth v0.18.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.21.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.6.0 // indirect
	github.com/GehirnInc/crypt v0.0.0-20230320061759-8cc1b52080c5 // indirect
	github.com/IBM/sarama v1.48.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antchfx/xmlquery v1.5.1 // indirect
	github.com/antchfx/xpath v1.3.6 // indirect
	github.com/apache/thrift v0.23.0 // indirect
	github.com/aws/aws-msk-iam-sasl-signer-go v1.0.4 // indirect
	github.com/aws/aws-sdk-go-v2 v1.41.6 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.16 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.15 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.0 // indirect
	github.com/aws/smithy-go v1.25.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/dgraph-io/ristretto/v2 v2.2.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/eapache/go-resiliency v1.7.0 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/elastic/elastic-transport-go/v8 v8.8.0 // indirect
	github.com/elastic/go-grok v0.3.1 // indirect
	github.com/elastic/lunes v0.2.0 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/foxboron/go-tpm-keyfiles v0.0.0-20251226215517-609e4778396f // indirect
	github.com/go-faster/city v1.0.1 // indirect
	github.com/go-faster/errors v0.7.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/flatbuffers v25.2.10+incompatible // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.14 // indirect
	github.com/googleapis/gax-go/v2 v2.18.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.3.4 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/internal/credentialsfile v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/healthcheck v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/kafka v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/pdatautil v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/batchpersignal v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/kafka/configkafka v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/kafka/topic v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/sampling v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/status v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/azure v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.151.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin v0.151.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/paulmach/orb v0.12.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_golang/exp v0.0.0-20260325093428-d8591d0db856 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/prometheus/prometheus v0.311.3 // indirect
	github.com/prometheus/sigv4 v0.4.1 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9 // indirect
	github.com/relvacode/iso8601 v1.7.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/shirou/gopsutil/v4 v4.26.3 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tg123/go-htpasswd v1.2.4 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/twmb/franz-go v1.20.7 // indirect
	github.com/twmb/franz-go/pkg/kadm v1.17.2 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.13.1 // indirect
	github.com/twmb/franz-go/pkg/sasl/kerberos v1.1.0 // indirect
	github.com/twmb/franz-go/plugin/kzap v1.1.2 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/ua-parser/uap-go v0.0.0-20251207011819-db9adb27a0b8 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/collector v0.151.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.57.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v1.57.0 // indirect
	go.opentelemetry.io/collector/connector/connectortest v0.151.0 // indirect
	go.opentelemetry.io/collector/connector/xconnector v0.151.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.151.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.151.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.151.0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.151.0 // indirect
	go.opentelemetry.io/collector/exporter/xexporter v0.151.0 // indirect
	go.opentelemetry.io/collector/extension/extensionmiddleware v0.151.0 // indirect
	go.opentelemetry.io/collector/extension/extensiontest v0.151.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.151.0 // indirect
	go.opentelemetry.io/collector/internal/componentalias v0.151.0 // indirect
	go.opentelemetry.io/collector/internal/fanoutconsumer v0.151.0 // indirect
	go.opentelemetry.io/collector/internal/memorylimiter v0.151.0 // indirect
	go.opentelemetry.io/collector/internal/sharedcomponent v0.151.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.151.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.151.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.151.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.151.0 // indirect
	go.opentelemetry.io/collector/processor/processorhelper/xprocessorhelper v0.151.0 // indirect
	go.opentelemetry.io/collector/processor/xprocessor v0.151.0 // indirect
	go.opentelemetry.io/collector/receiver/receiverhelper v0.151.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.151.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.151.0 // indirect
	go.opentelemetry.io/collector/service/hostcapabilities v0.151.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.18.0 // indirect
	go.opentelemetry.io/contrib/otelconf v0.23.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.43.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.68.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.19.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.19.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.19.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.43.0 // indirect
	go.opentelemetry.io/otel/log v0.19.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.19.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/api v0.272.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260406210006-6f92a3bedf2d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260406210006-6f92a3bedf2d // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apimachinery v0.35.3 // indirect
	k8s.io/client-go v0.35.3 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4 // indirect
)
