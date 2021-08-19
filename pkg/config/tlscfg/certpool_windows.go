// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows
// +build windows

package tlscfg

import (
	"crypto/x509"
	"syscall"
	"unsafe"
)

const (
	// CRYPT_E_NOT_FOUND is an error code specific to windows cert pool.
	// See https://github.com/golang/go/issues/16736#issuecomment-540373689.
	CRYPT_E_NOT_FOUND = 0x80092004
)

// workaround https://github.com/golang/go/issues/16736
// fix borrowed from Sensu: https://github.com/sensu/sensu-go/pull/4018
func appendCerts(rootCAs *x509.CertPool) (*x509.CertPool, error) {
	storeHandle, err := syscall.CertOpenSystemStore(0, syscall.StringToUTF16Ptr("Root"))
	if err != nil {
		return nil, err
	}

	var cert *syscall.CertContext
	for {
		cert, err = syscall.CertEnumCertificatesInStore(storeHandle, cert)
		if err != nil {
			if errno, ok := err.(syscall.Errno); ok {
				if errno == CRYPT_E_NOT_FOUND {
					break
				}
			}
			return nil, err
		}
		if cert == nil {
			break
		}
		// Copy the buf, since ParseCertificate does not create its own copy.
		buf := (*[1 << 20]byte)(unsafe.Pointer(cert.EncodedCert))[:]
		buf2 := make([]byte, cert.Length)
		copy(buf2, buf)
		if c, err := x509.ParseCertificate(buf2); err == nil {
			rootCAs.AddCert(c)
		}
	}
	return rootCAs, nil
}

func loadSystemCertPool() (*x509.CertPool, error) {
	certPool, err := systemCertPool()
	if err != nil {
		return nil, err
	}
	if certPool == nil {
		certPool = x509.NewCertPool()
	}
	return appendCerts(certPool)
}
