package depstore

import "github.com/jaegertracing/jaeger/storage_v2"

type Factory interface {
	storage_v2.FactoryBase
	CreateDependencyReader() (Reader, error)
}
