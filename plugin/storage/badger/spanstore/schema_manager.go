package spanstore

import (
	"bytes"
	"encoding/binary"

	"github.com/dgraph-io/badger"
	"github.com/golang/protobuf/proto"
	"github.com/jaegertracing/jaeger/model"
	"go.uber.org/zap"
)

const (
	metadataRange     byte   = 0x1F
	schemaVersionKey  byte   = 0x11
	currentVersion    uint32 = 1
	spanKeyPrefixVer0 byte   = 0x80
	indexKeyRangeVer0 byte   = 0x0F
	protoEncodingVer0 byte   = 0x02 // Ver0 was shipped with only protoEncoding allowed
	depIndexKeyVer1   byte   = 0x85
)

/*
	Methods here might reuse code that's already in TraceReader or SpanWriter. We can't use that code,
	since it might be developed for newer schema versions and as such will not necessarily work correctly
	with the older code.
*/

// SchemaUpdate reads the existing schema version and updates accordingly
func SchemaUpdate(store *badger.DB, logger *zap.Logger) error {
	schemaVersion := currentVersion

	err := store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		schemaKey := []byte{schemaVersionKey & metadataRange}

		it.Seek(schemaKey)
		if it.Item() != nil && bytes.Equal(schemaKey, it.Item().Key()) {
			val, err := it.Item().Value()
			if err != nil {
				return err
			}
			schemaVersion = binary.BigEndian.Uint32(val)
			return nil
		}
		// No key found, meaning we're running the original version or empty storage
		spanKey := []byte{spanKeyPrefixVer0}
		it.Seek(spanKey)
		if it.Item() != nil && bytes.HasPrefix(spanKey, it.Item().Key()) {
			schemaVersion = 0
		}
		// Empty database
		return nil
	})

	if err != nil {
		return err
	}

	switch schemaVersion {
	case 0:
		// Update to 1
		logger.Info("Updating badger storage schema from version 0 to version 1")
		err = updateTo1(store)
		if err != nil {
			return err
		}
		fallthrough
	default:
		err = setSchemaVersion(store, currentVersion)
		break
	}

	return err
}

// updateTo1 merges the data store from schema version 0 to schema version 1. The change is an added index for dependency calculations.
// Thus the method needs to read all the spans and write a new index key for each.
func updateTo1(store *badger.DB) error {
	prefix := []byte{spanKeyPrefixVer0}

	err := store.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		val := []byte{}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// Add value to the span store (decode from JSON / defined encoding first)
			// These are in the correct order because of the sorted nature
			item := it.Item()
			val, err := item.ValueCopy(val)
			if err != nil {
				return err
			}

			sp := model.Span{}
			if err := proto.Unmarshal(val, &sp); err != nil {
				return err
			}

			newKey := createDependencyIndexKeyVer1(&sp)
			txn.SetEntry(&badger.Entry{
				Key:       newKey,
				Value:     nil,
				ExpiresAt: item.ExpiresAt(), // Same expiration time as the span we read in
			})
		}
		return nil
	})

	return err
}

// setSchemaVersion writes the version tag to the database
func setSchemaVersion(store *badger.DB, version uint32) error {
	key := []byte{schemaVersionKey & metadataRange}
	value := make([]byte, 4)
	binary.BigEndian.PutUint32(value, version)

	store.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, value)
		if err != nil {
			return err
		}
		return nil
	})
	return nil
}

func createDependencyIndexKeyVer1(span *model.Span) []byte {
	// I need (for sorting purposes and optimization of reads):
	// depIndex<traceId><startTime><spanId><serviceName><parentSpanId> (if parentSpanId exists)

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
