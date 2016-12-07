package model

// Process describes an instance of an application or service that emits tracing data.
type Process struct {
	ServiceName string `json:"serviceName"`
	Tags        []Tag  `json:"tags,omitempty"`
}

// NewProcess creates a new Process for given serviceName and tags.
// The tags are sorted in place and kept in the the same array/slice.
func NewProcess(serviceName string, tags []Tag) *Process {
	SortTags(tags)
	return &Process{ServiceName: serviceName, Tags: tags}
}
