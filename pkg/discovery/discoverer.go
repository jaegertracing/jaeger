package discovery

// Discoverer listens to a service discovery system and yields a set of
// identical instance locations. An error indicates a problem with connectivity
// to the service discovery system, or within the system itself; a subscriber
// may yield no endpoints without error.
type Discoverer interface {
	Instances() ([]string, error)
}

// FixedDiscoverer yields a fixed set of instances.
type FixedDiscoverer []string

// Instances implements Discoverer.
func (d FixedDiscoverer) Instances() ([]string, error) { return d, nil }
