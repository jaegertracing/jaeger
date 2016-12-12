package multierror

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func ExampleWrap() {
	someFunc := func() error {
		return errors.New("doh")
	}

	var errs []error
	for i := 0; i < 2; i++ {
		if err := someFunc(); err != nil {
			errs = append(errs, err)
		}
		fmt.Println(Wrap(errs).Error())
	}
	// Output: doh
	// [doh, doh]
}

func TestWrapEmptySlice(t *testing.T) {
	var errors []error
	e1 := Wrap(errors)
	assert.Nil(t, e1)
	e2 := Wrap([]error{})
	assert.Nil(t, e2)
}

func TestWrapSingleError(t *testing.T) {
	err := errors.New("doh")
	e1 := Wrap([]error{err})
	assert.Error(t, e1)
	assert.Equal(t, err, e1)
	assert.Equal(t, "doh", e1.Error())
}

func TestWrapManyErrors(t *testing.T) {
	err1 := errors.New("ay")
	err2 := errors.New("caramba")
	e1 := Wrap([]error{err1, err2})
	assert.Error(t, e1)
	assert.Equal(t, "[ay, caramba]", e1.Error())
}
