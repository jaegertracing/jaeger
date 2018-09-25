package main

import (
	"flag"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
)

type preSavePlugin struct {
	logger   *zap.Logger
	interval int
	counter  int
}

var (
	p = preSavePlugin{}
)

const (
	prefix      = "pre-save-"
	logInterval = prefix + "log-interval"
)

func (p *preSavePlugin) AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(logInterval, 30, "How often should the PreSave Logger should print")
}

func (p *preSavePlugin) InitFromViper(v *viper.Viper) {
	p.interval = v.GetInt(logInterval)
	p.startLogger()
}

func (p *preSavePlugin) AddLogger(logger *zap.Logger) {
	p.logger = logger
}

func (p *preSavePlugin) AddMetrics(metricsFactory metrics.Factory) {

}

func (p *preSavePlugin) preSave(span *model.Span) {
	p.counter++
}

func (p *preSavePlugin) startLogger() {
	ticker := time.NewTicker(time.Duration(p.interval) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				p.logger.Info("PreSave Logger",
					zap.Int("rate", p.counter/p.interval),
					zap.Int("count", p.counter),
					zap.Int("interval", p.interval))
				p.counter = 0
			}
		}
	}()
}

// Export the needed symbols
var Configurable plugin.Configurable = &p
var PreSave app.ProcessSpan = p.preSave
