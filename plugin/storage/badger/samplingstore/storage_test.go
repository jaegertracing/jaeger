package samplingstore

import (
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	samplemodel "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/stretchr/testify/assert"
)

type samplingStoreTest struct {
	store *SamplingStore
}

func NewtestSamplingStore(db *badger.DB) *samplingStoreTest {
	return &samplingStoreTest{
		store: NewSamplingStore(db),
	}
}

func TestInsertThroughput(t *testing.T) {
	runWithBadger(t, func(s *samplingStoreTest, t *testing.T) {
		throughputs := []*samplemodel.Throughput{
			{Service: "my-svc", Operation: "op"},
			{Service: "our-svc", Operation: "op2"},
		}
		err := s.store.InsertThroughput(throughputs)
		assert.NoError(t, err)
	})
}

func TestGetThroughput(t *testing.T) {
	runWithBadger(t, func(s *samplingStoreTest, t *testing.T) {
		start := time.Now()

		expected := 2
		throughputs := []*samplemodel.Throughput{
			{Service: "my-svc", Operation: "op"},
			{Service: "our-svc", Operation: "op2"},
		}
		err := s.store.InsertThroughput(throughputs)
		assert.NoError(t, err)

		actual, err := s.store.GetThroughput(start, start.Add(time.Second*time.Duration(10)))
		assert.NoError(t, err)
		assert.Equal(t, expected, len(actual))
	})
}

func runWithBadger(t *testing.T, test func(s *samplingStoreTest, t *testing.T)) {
	opts := badger.DefaultOptions("")

	opts.SyncWrites = false
	dir := t.TempDir()
	opts.Dir = dir
	opts.ValueDir = dir

	store, err := badger.Open(opts)
	defer func() {
		store.Close()
	}()
	ss := NewtestSamplingStore(store)

	assert.NoError(t, err)

	test(ss, t)
}
