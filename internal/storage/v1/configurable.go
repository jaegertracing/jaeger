// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Configurable interface can be implemented by plugins that require external configuration,
// such as CLI flags, config files, or environment variables.
type Configurable interface {
	// AddFlags adds CLI flags for configuring this component.
	AddFlags(flagSet *flag.FlagSet)

	// InitFromViper initializes this component with properties from spf13/viper.
	InitFromViper(v *viper.Viper, logger *zap.Logger)
}
