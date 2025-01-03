package cleaner

import (
	"time"

	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/filter"
	"go.uber.org/zap"
)

type IndexCleaner struct {
	// client is the client used for interacting with es
	client *client.IndicesClient
	logger *zap.Logger
	// indexPrefix is the user-defined index prefix, required for searching indices
	indexPrefix string
	// timePeriod is the interval at which cleaner would run
	// also, cleaning is done for index with creationTime < now - timePeriod
	timePeriod time.Duration
	ticker     *time.Ticker
}

func NewIndexCleaner(client *client.IndicesClient, logger *zap.Logger, indexPrefix string, timePeriod time.Duration) *IndexCleaner {
	return &IndexCleaner{
		client:      client,
		logger:      logger,
		indexPrefix: indexPrefix,
		timePeriod:  timePeriod,
		ticker:      time.NewTicker(timePeriod),
	}
}

func (i *IndexCleaner) Start() {
	go func() {
		for _ = range i.ticker.C {
			err := i.Clean()
			if err != nil {
				i.logger.Error("Index cleaning failed", zap.Error(err))
			}
		}
	}()
}

func (i *IndexCleaner) Clean() error {
	indices, err := i.client.GetJaegerIndices(i.indexPrefix)
	if err != nil {
		return err
	}

	year, month, day := time.Now().UTC().Date()
	tomorrowMidnight := time.Date(year, month, day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	deleteIndicesBefore := tomorrowMidnight.Add(-1 * i.timePeriod)
	i.logger.Info("Indices before this date will be deleted", zap.String("date", deleteIndicesBefore.Format(time.RFC3339)))

	indices = filter.ByDate(indices, deleteIndicesBefore)

	if len(indices) == 0 {
		i.logger.Info("No indices to delete")
		return nil
	}

	i.logger.Info("Deleting indices", zap.Any("indices", indices))
	return i.client.DeleteIndices(indices)
}

func (i *IndexCleaner) Stop() {
	i.ticker.Stop()
}
