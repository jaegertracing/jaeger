package authorizingproxy

import (
  "fmt"
  "log"
  "net"
  "net/http"
  "strconv"
  "testing"
  "time"

  "github.com/gorilla/mux"
  "github.com/spf13/viper"
  "github.com/uber/tchannel-go"
  "github.com/uber/tchannel-go/thrift"
  "go.uber.org/zap"

  "github.com/uber/jaeger-lib/metrics"

  collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
  agentReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
  basicB "github.com/jaegertracing/jaeger/cmd/builder"
  "github.com/jaegertracing/jaeger/cmd/collector/app/builder"
  "github.com/jaegertracing/jaeger/cmd/flags"
  "github.com/jaegertracing/jaeger/plugin/storage"
  pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
  "github.com/jaegertracing/jaeger/pkg/healthcheck"
  "github.com/jaegertracing/jaeger/storage/spanstore"
  "github.com/jaegertracing/jaeger/pkg/recoveryhandler"

  "github.com/jaegertracing/jaeger/pkg/discovery"

  jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
  zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"

  jaegerThrift "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

var (
  CollectorTChannelPort = 14267
  CollectorHTTPPort = 14268
  CollectorZipkinHTTPPort = 14269
)

func startCollector() spanstore.Writer {

  serviceName := "jaeger-collector"

  v := viper.New()
  v.Set("collector.host-port", "127.0.0.1:" + strconv.Itoa(CollectorTChannelPort))
  v.Set("authorizingproxy.proxy-hostport", "127.0.0.1:10000")

  builderOpts := new(builder.CollectorOptions).InitFromViper(v)

  storageFactory, err := storage.NewFactory(storage.FactoryConfig{ "authorizing_proxy", "authorizing_proxy" })
  if err != nil {
    log.Fatalf("Cannot initialize storage factory: %v", err)
    return nil
  }

  sFlags := new(flags.SharedFlags).InitFromViper(v)
  logger, err := sFlags.NewLogger(zap.NewProductionConfig())
  if err != nil {
    return nil
  }

  hc, err := healthcheck.
    New(healthcheck.Unavailable, healthcheck.Logger(logger)).
    Serve(builderOpts.CollectorHealthCheckHTTPPort)
  if err != nil {
    logger.Fatal("Could not start the health check server.", zap.Error(err))
  }

  mBldr := new(pMetrics.Builder).InitFromViper(v)
  //metricsFactory, err := mBldr.CreateMetricsFactory("jaeger-collector")
  //if err != nil {
  //  logger.Fatal("Cannot create metrics factory.", zap.Error(err))
  //  return nil
  //}

  storageFactory.InitFromViper(v)
  if err := storageFactory.Initialize(metrics.NullFactory, logger); err != nil {
    logger.Fatal("Failed to init storage factory", zap.Error(err))
    return nil
  }

  spanWriter, err := storageFactory.CreateSpanWriter()
  if err != nil {
    logger.Fatal("Failed to create span writer", zap.Error(err))
    return nil
  }

  handlerBuilder, err := builder.NewSpanHandlerBuilder(
    builderOpts,
    spanWriter,
    basicB.Options.LoggerOption(logger),
    basicB.Options.MetricsFactoryOption(metrics.NullFactory),
  )
  if err != nil {
    logger.Fatal("Unable to set up builder", zap.Error(err))
    return nil
  }

  ch, err := tchannel.NewChannel(serviceName, &tchannel.ChannelOptions{})
  if err != nil {
    logger.Fatal("Unable to create new TChannel", zap.Error(err))
    return nil
  }
  server := thrift.NewServer(ch)
  zipkinSpansHandler, jaegerBatchesHandler := handlerBuilder.BuildHandlers()
  server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
  server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))

  portStr := ":" + strconv.Itoa(CollectorTChannelPort)
  listener, err := net.Listen("tcp", portStr)
  if err != nil {
    logger.Fatal("Unable to start listening on channel", zap.Error(err))
    return nil
  }
  ch.Serve(listener)

      r := mux.NewRouter()
      apiHandler := collectorApp.NewAPIHandler(jaegerBatchesHandler)
      apiHandler.RegisterRoutes(r)
      if h := mBldr.Handler(); h != nil {
        logger.Info("Registering metrics handler with HTTP server", zap.String("route", mBldr.HTTPRoute))
        r.Handle(mBldr.HTTPRoute, h)
      }
      httpPortStr := ":" + strconv.Itoa(CollectorHTTPPort)
      recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)

      //go startZipkinHTTPAPI(logger, CollectorZipkinHTTPPort, zipkinSpansHandler, recoveryHandler)

      logger.Info("Starting Jaeger Collector HTTP server", zap.Int("http-port", CollectorHTTPPort))

      go func() {
        if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
          logger.Fatal("Could not launch service", zap.Error(err))
        }
        hc.Set(healthcheck.Unavailable)
      }()

      hc.Ready()

  return spanWriter
}

func createAgentReporter() (*agentReporter.Reporter, error) {
  v := viper.New()
  sFlags := new(flags.SharedFlags).InitFromViper(v)
  logger, err := sFlags.NewLogger(zap.NewProductionConfig())
  if err != nil {
    return nil, err
  }

  discoverer := discovery.FixedDiscoverer([]string{ "127.0.0.1:" + strconv.Itoa(CollectorTChannelPort) })
  notifier := &discovery.Dispatcher{}

  builder := agentReporter.NewBuilder()
  builder.WithDiscoverer(discoverer)
  builder.WithDiscoveryNotifier(notifier)

  return builder.CreateReporter(metrics.NullFactory, logger)

}

func strToRef(s string) *string {
  return &s
}

func TestBasics(t *testing.T) {

  spanWriter := startCollector()
  if spanWriter == nil {
    t.Error("Expected factory to be not nil")
  }

  reporter, err := createAgentReporter()
  if err != nil {
    t.Error(fmt.Sprintf("%+v", err))
  }

  batch := &jaegerThrift.Batch{}
  batch.Process = &jaegerThrift.Process{
    "integration-test-service",
    make([]*jaegerThrift.Tag, 0) }

  span := jaegerThrift.Span{
    int64(0),
    int64(1),
    int64(100),
    int64(0),
    "integration-test-operation",
    make([]*jaegerThrift.SpanRef, 0),
    int32(0),
    int64(1234567890),
    int64(1000),
    make([]*jaegerThrift.Tag, 0),
    []*jaegerThrift.Log{
      &jaegerThrift.Log{
        int64(1234567890),
        []*jaegerThrift.Tag{
          &jaegerThrift.Tag{
            Key: "event",
            VType: jaegerThrift.TagType_STRING,
            VStr: strToRef("baggage"),
          },
          &jaegerThrift.Tag{
            Key: "key",
            VType: jaegerThrift.TagType_STRING,
            VStr: strToRef("x-klarrio-auth-key"),
          },
          &jaegerThrift.Tag{
            Key: "value",
            VType: jaegerThrift.TagType_STRING,
            VStr: strToRef("mmmm"),
          },
        },
      },
    },
  }
  batch.Spans = []*jaegerThrift.Span{ &span }

  reporter.EmitBatch(batch)

  time.Sleep(time.Duration(5) * time.Second)

}