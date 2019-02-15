package spanstore

import (
	"hash"
	"math"
)

// HashBytes returns the uint64 hash value of bytes slice
func HashBytes(h hash.Hash64, bytes []byte) uint64 {
	h.Reset()
	_, err := h.Write(bytes)
	if err != nil {
		// No downsampling when there's error hashing
		return math.MaxUint64
	}
	return h.Sum64()
}
