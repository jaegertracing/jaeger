// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage"
)

// Configurable interface can be implemented by plugins that require external configuration,
// such as CLI flags, config files, or environment variables.
type Configurable interface {
	// AddFlags adds CLI flags for configuring this component.
	AddFlags(flagSet *flag.FlagSet)

	// InitFromViper initializes this component with properties from spf13/viper.
	InitFromViper(v *viper.Viper, logger *zap.Logger)
}

// DefaultConfigurable is an interface that can be implement by some storage implementations
// to provide a way to inherit configuration settings from another factory.
type DefaultConfigurable interface {
	InheritSettingsFrom(other storage.Factory)
}
