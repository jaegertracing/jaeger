package mocks

import "github.com/stretchr/testify/mock"

type Lock struct {
	mock.Mock
}

func (_m *Lock) Acquire(resource string) (bool, error) {
	ret := _m.Called(resource)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string) bool); ok {
		r0 = rf(resource)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(resource)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
