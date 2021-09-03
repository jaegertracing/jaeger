package app

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/es/client"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func newESClient(endpoint string, cfg *Config, tlsCfg *tls.Config) client.Client {
	httpClient := &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: tlsCfg,
		},
	}
	return client.Client{
		Endpoint:  endpoint,
		Client:    httpClient,
		BasicAuth: client.BasicAuth(cfg.Username, cfg.Password),
	}
}

type Action interface {
	Do() error
}

type ActionExecuteOptions struct {
	Args     []string
	Viper    *viper.Viper
	Logger   *zap.Logger
	Config   Config
	TlsFlags tlscfg.ClientFlagsConfig
}

type ActionCreatorFunction func(client.Client) Action

func ExecuteAction(opts ActionExecuteOptions, createAction ActionCreatorFunction) error {
	if len(opts.Args) != 1 {
		return fmt.Errorf("wrong number of arguments")
	}

	tlsOpts := opts.TlsFlags.InitFromViper(opts.Viper)
	tlsCfg, err := tlsOpts.Config(opts.Logger)
	if err != nil {
		return err
	}
	defer tlsOpts.Close()

	esClient := newESClient(opts.Args[0], &opts.Config, tlsCfg)
	action := createAction(esClient)
	return action.Do()
}
