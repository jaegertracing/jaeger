// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"errors"
	"fmt"

	"github.com/olivere/elastic"
)

// DetailedError creates a more detailed error if the error stack contains elastic.Error.
// This is useful because by default olivere/elastic returns errors that print like this:
//
//	elastic: Error 400 (Bad Request): all shards failed [type=search_phase_execution_exception]
//
// This is pretty useless because it masks the underlying root cause.
// DetailedError would instead return an error like this:
//
//	<same as above>: RootCause[... detailed error message ...]
func DetailedError(err error) error {
	var esErr *elastic.Error
	if errors.As(err, &esErr) {
		if esErr.Details != nil && len(esErr.Details.RootCause) > 0 {
			rc := esErr.Details.RootCause[0]
			if rc != nil {
				return fmt.Errorf("%w: RootCause[%s [type=%s]]", err, rc.Reason, rc.Type)
			}
		}
	}
	return err
}
