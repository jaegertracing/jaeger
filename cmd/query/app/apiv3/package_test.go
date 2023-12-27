// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	if true {
		goleak.VerifyTestMain(m)
	}
}
