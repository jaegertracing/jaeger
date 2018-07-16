package adaptive

// samplingCacheEntry keeps track of the probability and whether a service-operation is using adaptive sampling
type samplingCacheEntry struct {
	probability    float64
	usingAdapative bool
}

type samplingCache map[string]map[string]*samplingCacheEntry

func (s samplingCache) Set(service, operation string, entry *samplingCacheEntry) {
	if _, ok := s[service]; !ok {
		s[service] = make(map[string]*samplingCacheEntry)
	}
	s[service][operation] = entry
}

func (s samplingCache) Get(service, operation string) *samplingCacheEntry {
	_, ok := s[service]
	if !ok {
		return nil
	}
	return s[service][operation]
}
