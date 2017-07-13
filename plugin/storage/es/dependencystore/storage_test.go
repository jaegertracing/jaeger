package dependencystore

import (
	"github.com/uber/jaeger/pkg/es/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"go.uber.org/zap"
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/storage/dependencystore"
)

type storageTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	storage   *DependencyStore
}

func withStorage(fn func(r *storageTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	r := &storageTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		storage:    NewDependencyStore(client, logger),
	}
	fn(r)
}

func TestNewStorage(t *testing.T) {
	withStorage(func(r *storageTest) {
		var reader dependencystore.Reader = r.storage // check API conformance
		var writer dependencystore.Writer = r.storage // check API conformance
		assert.NotNil(t, reader)
		assert.NotNil(t, writer)
	})
}
