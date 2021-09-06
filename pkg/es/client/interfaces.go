package client

type IndexAPI interface {
	GetJaegerIndices(prefix string) ([]Index, error)
	DeleteIndices(indices []Index) error
	CreateIndex(index string) error
	CreateAlias(aliases []Alias) error
	DeleteAlias(aliases []Alias) error
	CreateTemplate(template, name string) error
	Rollover(rolloverTarget string, conditions map[string]interface{}) error
}

type ClusterAPI interface {
	Version() (uint, error)
}

type IndexManagementLifecycleAPI interface {
	Exists(name string) (bool, error)
}
