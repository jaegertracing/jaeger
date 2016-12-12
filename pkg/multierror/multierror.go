package multierror

import (
	"fmt"
	"strings"
)

// Wrap takes an array of errors and returns a single error that encapsulates
// those underlying errors. If the array is nil or empty it returns nil.
// If the array only contains a single element, that error is returned directly.
func Wrap(errs []error) error {
	return multiError(errs).AsError()
}

// Errors bundles more than one error together into a single error.
type multiError []error

// AsError returns either: nil, the only error, or the Error instance itself
// if there are 0, 1, or more errors in the slice respectively.  This method is
// useful for contexts that want to do a simple return Errors(errors).AsError().
func (errors multiError) AsError() error {
	switch len(errors) {
	case 0:
		return nil
	case 1:
		return errors[0]
	default:
		return errors
	}
}

// Error returns a string like "[e1, e2, ...]" where each eN is the Error() of
// each error in the slice.
func (errors multiError) Error() string {
	parts := make([]string, len(errors))
	for i, err := range errors {
		parts[i] = err.Error()
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}
