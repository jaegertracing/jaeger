package spanstore

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/gogo/protobuf/proto"
	"github.com/jaegertracing/jaeger/model"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

/*
	Tests in read_write_test.go already check the empty db state - no need to retest that behavior here.
*/

func TestSchemaMigrate(t *testing.T) {
	runWithBadger(t, func(store *badger.DB, t *testing.T) {
		// Write Ver0 data (not everything, but important parts)
		testSpan := createDummySpan()
		key, value, err := createVer0Span(testSpan)
		assert.NoError(t, err)

		err = store.Update(func(txn *badger.Txn) error {
			err := txn.Set(key, value)
			return err
		})

		assert.NoError(t, err)

		// Migrate data
		err = SchemaUpdate(store, zap.NewNop())
		assert.NoError(t, err)

		err = store.View(func(txn *badger.Txn) error {
			// Check existence of schema key (ver1)
			schemaKey := []byte{0x11}
			item, err := txn.Get(schemaKey)
			assert.NoError(t, err)

			val, err := item.Value()
			assert.NoError(t, err)

			schemaVersion := int(binary.BigEndian.Uint32(val))
			assert.Equal(t, 1, schemaVersion)

			// Verify existence of dependencykey (ver1)
			depKey := createVer1DepKey(testSpan)
			item, err = txn.Get(depKey)
			assert.NoError(t, err)
			return nil
		})
		assert.NoError(t, err)
	})
}

func createVer0Span(span model.Span) ([]byte, []byte, error) {
	buf := new(bytes.Buffer)

	buf.WriteByte(spanKeyPrefix)
	binary.Write(buf, binary.BigEndian, span.TraceID.High)
	binary.Write(buf, binary.BigEndian, span.TraceID.Low)
	binary.Write(buf, binary.BigEndian, model.TimeAsEpochMicroseconds(span.StartTime))
	binary.Write(buf, binary.BigEndian, span.SpanID)

	var bb []byte
	var err error

	bb, err = proto.Marshal(&span)

	return buf.Bytes(), bb, err

}

func createVer1DepKey(span model.Span) []byte {
	buf := new(bytes.Buffer)

	buf.WriteByte((depIndexKeyVer1 & indexKeyRangeVer0) | spanKeyPrefixVer0)
	binary.Write(buf, binary.BigEndian, span.TraceID.High)
	binary.Write(buf, binary.BigEndian, span.TraceID.Low)
	binary.Write(buf, binary.BigEndian, model.TimeAsEpochMicroseconds(span.StartTime))
	binary.Write(buf, binary.BigEndian, span.SpanID)
	binary.Write(buf, binary.BigEndian, []byte(span.Process.ServiceName))
	binary.Write(buf, binary.BigEndian, span.ParentSpanID())

	return buf.Bytes()
}
