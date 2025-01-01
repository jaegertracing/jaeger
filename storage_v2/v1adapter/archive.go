package v1adapter

import (
	"errors"

	"github.com/jaegertracing/jaeger/storage"
	"go.uber.org/zap"
)

func InitializeArchiveStorage(
	storageFactory storage.BaseFactory,
	logger *zap.Logger,
) (*TraceReader, *TraceWriter) {
	archiveFactory, ok := storageFactory.(storage.ArchiveFactory)
	if !ok {
		logger.Info("Archive storage not supported by the factory")
		return nil, nil
	}
	reader, err := archiveFactory.CreateArchiveSpanReader()
	if errors.Is(err, storage.ErrArchiveStorageNotConfigured) || errors.Is(err, storage.ErrArchiveStorageNotSupported) {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return nil, nil
	}
	if err != nil {
		logger.Error("Cannot init archive storage reader", zap.Error(err))
		return nil, nil
	}
	writer, err := archiveFactory.CreateArchiveSpanWriter()
	if errors.Is(err, storage.ErrArchiveStorageNotConfigured) || errors.Is(err, storage.ErrArchiveStorageNotSupported) {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return nil, nil
	}
	if err != nil {
		logger.Error("Cannot init archive storage writer", zap.Error(err))
		return nil, nil
	}

	return NewTraceReader(reader), NewTraceWriter(writer)
}
