package app

import (
	"go.uber.org/zap"

	istorage "github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
)

// ArchiveOptions returns an instance of QueryServiceOptions based on readers/writers created from storageFactory
func ArchiveOptions(storageFactory istorage.Factory, logger *zap.Logger) querysvc.QueryServiceOptions {
	archiveFactory, ok := storageFactory.(istorage.ArchiveFactory)
	if !ok {
		logger.Info("Archive storage not supported by the factory")
		return querysvc.QueryServiceOptions{}
	}
	reader, err := archiveFactory.CreateArchiveSpanReader()
	if err == istorage.ErrArchiveStorageNotConfigured || err == istorage.ErrArchiveStorageNotSupported {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return querysvc.QueryServiceOptions{}
	}
	if err != nil {
		logger.Error("Cannot init archive storage reader", zap.Error(err))
		return querysvc.QueryServiceOptions{}
	}
	writer, err := archiveFactory.CreateArchiveSpanWriter()
	if err == istorage.ErrArchiveStorageNotConfigured || err == istorage.ErrArchiveStorageNotSupported {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return querysvc.QueryServiceOptions{}
	}
	if err != nil {
		logger.Error("Cannot init archive storage writer", zap.Error(err))
		return querysvc.QueryServiceOptions{}
	}
	return querysvc.QueryServiceOptions {
		ArchiveSpanReader: reader,
		ArchiveSpanWriter: writer,
	}
}