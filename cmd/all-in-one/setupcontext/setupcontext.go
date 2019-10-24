package setupcontext

import "testing"

var isAllInOne bool

// SetAllInOne sets the internal flag to all in one on.
func SetAllInOne() {
	isAllInOne = true
}

// UnsetAllInOne unsets the internal all-in-one flag. Used in tests.
func UnsetAllInOne(t *testing.T) {
	isAllInOne = false
}

// IsAllInOne returns true when all in one mode is on.
func IsAllInOne() bool {
	return isAllInOne
}
