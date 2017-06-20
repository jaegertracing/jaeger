package distributedlock

// Lock uses distributed lock for control of a resource.
type Lock interface {
	// Acquire acquires a lock around a given resource.
	Acquire(resource string) (acquired bool, err error)

	// TODO add Forfeit to voluntarily give up the resource
}
