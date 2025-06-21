package elasticsearch

import (
	"errors"
	elasticv7 "github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// failingAggregation is a mock aggregation that fails when Source() is called
type failingAggregation struct{}

func (*failingAggregation) Source() (any, error) {
	return nil, errors.New("forced aggregation source error")
}

func TestBuildQueryJSON_ErrorCases(t *testing.T) {
	t.Run("failed to get query source", func(t *testing.T) {
		logger := QueryLogger{}

		_, err := logger.GetQueryJSON(elasticv7.NewBoolQuery(), &failingAggregation{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get query source")
	})
}
