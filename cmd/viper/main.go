package main

import (
	"flag"
	"strings"

	"fmt"

	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Builder struct {
	Processors []ProcessorConfiguration `yaml:"processors"`
	HTTPServer HTTPServerConfiguration  `yaml:"httpServer"`

	CollectorHostPorts []string `yaml:"collectorHostPorts"`
	DiscoveryMinPeers  int      `yaml:"minPeers"`
}

type ProcessorConfiguration struct {
	Workers  int                 `yaml:"workers"`
	Model    string              `yaml:"model"`
	Protocol string              `yaml:"protocol"`
	Server   ServerConfiguration `yaml:"server"`
}

type ServerConfiguration struct {
	QueueSize     int    `yaml:"queueSize"`
	MaxPacketSize int    `yaml:"maxPacketSize"`
	HostPort      string `yaml:"hostPort" validate:"nonzero"`
}

type HTTPServerConfiguration struct {
	HostPort string `yaml:"hostPort" validate:"nonzero"`
}

const (
	suffixDisabled            = "disabled"
	suffixWorkers             = "workers"
	suffixServerQueueSize     = "server-queue-size"
	suffixServerMaxPacketSize = "server-max-packet-size"
	suffixServerHostPort      = "server-host-port"
)

var processors = []struct {
	model    string
	protocol string
	port     string
}{
	{model: "zipkin", protocol: "binary", port: ":5775"},
	{model: "jaeger", protocol: "compact", port: ":6831"},
	{model: "jaeger", protocol: "binary", port: ":6832"},
}

func (b *Builder) getFlags() *flag.FlagSet {
	flagSet := &flag.FlagSet{}
	flagSet.String("collectorHostPorts", "", "collectorHostPorts")
	flagSet.String("httpServer.hostPort", ":5778", "http server host:port")
	flagSet.Int("minPeers", 3, "minPeers")

	for _, processor := range processors {
		prefix := "processor." + processor.model + "-" + processor.protocol + "."
		flagSet.Bool(prefix+suffixDisabled, false, "whether to disable "+processor.model+"-"+processor.protocol+" processor")
		flagSet.Int(prefix+suffixWorkers, 50, "num of workers")
		flagSet.Int(prefix+suffixServerQueueSize, 1000, "queue size")
		flagSet.Int(prefix+suffixServerMaxPacketSize, 65000, "num of workers")
		flagSet.String(prefix+suffixServerHostPort, processor.port, "host:port")
	}

	return flagSet
}

func (b *Builder) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	// logger.Info("config", zap.Any("keys", v.AllKeys()))
	b.DiscoveryMinPeers = v.GetInt("minPeers")
	b.CollectorHostPorts = strings.Split(v.GetString("collectorHostPorts"), ",")
	b.HTTPServer.HostPort = v.GetString("httpServer.hostPort")

	for _, processor := range processors {
		prefix := "processor." + processor.model + "-" + processor.protocol + "."
		if v.GetBool(prefix + suffixDisabled) {
			logger.Info("processor " + processor.model + "-" + processor.protocol + " is disabled")
			continue
		}
		p := &ProcessorConfiguration{Model: processor.model, Protocol: processor.protocol}
		p.initFromViper(v, prefix)
		b.Processors = append(b.Processors, *p)
	}
}

func (p *ProcessorConfiguration) initFromViper(v *viper.Viper, prefix string) {
	p.Workers = v.GetInt(prefix + suffixWorkers)
	p.Server.QueueSize = v.GetInt(prefix + suffixServerQueueSize)
	p.Server.MaxPacketSize = v.GetInt(prefix + suffixServerMaxPacketSize)
	p.Server.HostPort = v.GetString(prefix + suffixServerHostPort)
}

func main() {
	logger, _ := zap.NewProduction()
	b := &Builder{}

	v := viper.New()

	var command = &cobra.Command{
		Use:   "jaeger-agent",
		Short: "Jaeger agent is a local daemon program which collects tracing data.",
		Long: `Jaeger agent is a daemon program that runs on every host and receives
tracing data submitted by Jaeger client libraries.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Reading from config via Viper is possible but weird because the structure
			// needs to have elements like `zipkin-binary`.
			// TODO if we keep this, we need to demonstrate a sample config
			if config := v.GetString("config"); config != "" {
				logger.Info("reading config", zap.String("file", config))
				if reader, err := os.Open(config); err != nil {
					logger.Fatal("cannot open config file", zap.String("config", config), zap.Error(err))
				} else {
					defer reader.Close()
					if err := v.ReadConfig(reader); err != nil {
						logger.Fatal("cannot read config file", zap.String("config", config), zap.Error(err))
					}
				}
			}

			b.InitFromViper(v, logger)

			fmt.Printf("builder=%+v\n", b)

			return nil
		},
	}

	command.PersistentFlags().StringP("config", "f", "", "optional configuration file name")
	command.PersistentFlags().AddGoFlagSet(b.getFlags())

	v.BindPFlags(command.PersistentFlags())
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	if err := command.Execute(); err != nil {
		logger.Fatal("agent command failed", zap.Error(err))
	}

}
