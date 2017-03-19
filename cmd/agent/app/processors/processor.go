package processors

// Processor processes metrics in multiple formats
type Processor interface {
	Serve()
	Stop()
}
