// Copyright (c) 2017 Uber Technologies, Inc.
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

package metrics

import (
	"encoding/json"
	"errors"
	"expvar"
	"flag"
	"net/http"
	"time"

	gokitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/influx"
	influxcli "github.com/influxdata/influxdb/client/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	jexpvar "github.com/uber/jaeger-lib/metrics/expvar"
	"github.com/uber/jaeger-lib/metrics/go-kit"
	jinfl "github.com/uber/jaeger-lib/metrics/go-kit/influx"
	jprom "github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"
)

const (
	metricsBackend         = "metrics-backend"
	metricsHTTPRoute       = "metrics-http-route"
	metricsTags            = "metrics-tags"
	metricsInfluxServerURL = "metrics-influx-server-url"
	metricsInfluxDb        = "metrics-influx-db"
	metricsInfluxFrequency = "metrics-influx-reporting-frequency"
	metricsInfluxUser      = "metrics-influx-user"
	metricsInfluxPass      = "metrics-influx-pass"

	defaultMetricsBackend  = "prometheus"
	defaultMetricsRoute    = "/metrics"
	defaultMetricsTags     = "{}"
	defaultInfluxURL       = "http://localhost:8086"
	defaultInfluxDb        = "jaeger"
	defaultInfluxFrequency = "10s"
	defaultInfluxUser      = ""
	defaultInfluxPass      = ""
)

var errUnknownBackend = errors.New("unknown metrics backend specified")
var errMalformedMetricTags = errors.New("flag " + metricsTags + " is not a valid JSON map")

// Builder provides command line options to configure metrics backend used by Jaeger executables.
type Builder struct {
	Backend                  string
	HTTPRoute                string // endpoint name to expose metrics, e.g. for scraping
	tags                     map[string]string
	handler                  http.Handler
	influxServerURL          string
	influxDb                 string
	influxReportingFrequency time.Duration
	influxUser               string
	influxPass               string
}

// AddFlags adds flags for Builder.
func AddFlags(flags *flag.FlagSet) {
	flags.String(
		metricsBackend,
		defaultMetricsBackend,
		"Defines which metrics backend to use for metrics reporting: expvar, prometheus, influx, none")
	flags.String(
		metricsHTTPRoute,
		defaultMetricsRoute,
		"Defines the route of HTTP endpoint for metrics backends that support scraping")
	flags.String(
		metricsTags,
		defaultMetricsTags,
		`Tags reported with each metric. JSON-formatted map. E.g. {"tag1": "a", "tag2": "b"}`)
	flags.String(
		metricsInfluxServerURL,
		defaultInfluxURL,
		"Influx server address")
	flags.String(
		metricsInfluxDb,
		defaultInfluxDb,
		"The Influx database name")
	flags.String(
		metricsInfluxFrequency,
		defaultInfluxFrequency,
		"The frequency of reporting metrics to Influx")
	flags.String(
		metricsInfluxUser,
		defaultInfluxUser,
		"Influx username")
	flags.String(
		metricsInfluxPass,
		defaultInfluxPass,
		"Influx password")
}

// InitFromViper initializes Builder with properties retrieved from Viper.
func (b *Builder) InitFromViper(v *viper.Viper) (*Builder, error) {
	b.Backend = v.GetString(metricsBackend)
	b.HTTPRoute = v.GetString(metricsHTTPRoute)
	b.influxServerURL = v.GetString(metricsInfluxServerURL)
	b.influxDb = v.GetString(metricsInfluxDb)
	b.influxReportingFrequency = v.GetDuration(metricsInfluxFrequency)
	b.influxUser = v.GetString(metricsInfluxUser)
	b.influxPass = v.GetString(metricsInfluxPass)
	tagsStr := v.GetString(metricsTags)
	err := json.Unmarshal([]byte(tagsStr), &b.tags)
	if err != nil {
		return nil, errMalformedMetricTags
	}
	return b, nil
}

// CreateMetricsFactory creates a metrics factory based on the configured type of the backend.
// If the metrics backend supports HTTP endpoint for scraping, it is stored in the builder and
// can be later added by RegisterHandler function.
func (b *Builder) CreateMetricsFactory(namespace string, logger *zap.Logger) (metrics.Factory, error) {
	if b.Backend == "prometheus" {
		metricsFactory := jprom.New().Namespace(namespace, nil)
		b.handler = promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{DisableCompression: true})
		return metricsFactory, nil
	}
	if b.Backend == "expvar" {
		metricsFactory := jexpvar.NewFactory(10).Namespace(namespace, nil)
		b.handler = expvar.Handler()
		return metricsFactory, nil
	}
	if b.Backend == "influx" {
		reporter, err := influxcli.NewHTTPClient(influxcli.HTTPConfig{
			Addr:     b.influxServerURL,
			Username: b.influxUser,
			Password: b.influxPass,
		})
		if err != nil {
			return nil, err
		}

		inf := influx.New(b.tags, influxcli.BatchPointsConfig{
			Database: b.influxDb,
		}, gokitlog.NewNopLogger()) // NopLogger is ok because we use our own reporting loop with zap logger

		metricsFactory := xkit.Wrap(namespace, jinfl.NewFactory(inf))

		go func() {
			ticker := time.NewTicker(b.influxReportingFrequency)
			for range ticker.C {
				err := inf.WriteTo(reporter)
				if err != nil {
					logger.Warn("Failed to write to influx", zap.Error(err))
				}
			}
		}()
		logger.Info("Reporting metrics to influxdb",
			zap.String("url", b.influxServerURL),
			zap.String("username", b.influxUser),
			zap.String("db", b.influxDb),
			zap.Duration("frequency", b.influxReportingFrequency),
			zap.Reflect("tags", b.tags))
		return metricsFactory, nil
	}
	if b.Backend == "none" || b.Backend == "" {
		return metrics.NullFactory, nil
	}
	return nil, errUnknownBackend
}

// Handler returns an http.Handler for the metrics endpoint.
func (b *Builder) Handler() http.Handler {
	return b.handler
}
