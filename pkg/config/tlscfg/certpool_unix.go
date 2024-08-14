// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows
// +build !windows

package tlscfg

import (
	"crypto/x509"
)

func loadSystemCertPool() (*x509.CertPool, error) {
	return systemCertPool()
}
