// Copyright (c) 2018 The Jaeger Authors.
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

package throttling

import (
	"time"
)

// AccountConfig provides values to be used with an account object.
type AccountConfig struct {
	// MaxOperations defines the maximum number of operation specific token
	// buckets an account may maintain.
	MaxOperations int `yaml:"maxOperations"`
	// CreditsPerSecond defines the regeneration rate of the account's internal
	// token buckets.
	CreditsPerSecond float64 `yaml:"creditsPerSecond"`
	// MaxBalance defines the maximum amount of credits in a token bucket before
	// it will no longer accrue credits/tokens (until credits are used).
	MaxBalance float64 `yaml:"maxBalance"`
}

// ThrottlerConfig provides values to be used in a Throttler object.
type ThrottlerConfig struct {
	// DefaultAccountConfig defines the default AccountConfig to use for all
	// service accounts.
	DefaultAccountConfig AccountConfig `yaml:"defaultAccountConfig"`
	// AccountConfigOverrides overrides DefaultAccountConfig for services with
	// service names present in the map.
	AccountConfigOverrides map[string]*AccountConfig `yaml:"accountConfigOverrides"`

	// ClientMaxBalance defines the maximum balance a client may maintain before
	// further Withdraw calls return zero.
	ClientMaxBalance float64 `yaml:"clientMaxBalance"`

	// InactiveEntryLifetime defines the duration to await further updates
	// before purging entries. Applies to both accounts and clients.
	InactiveEntryLifetime time.Duration `yaml:"inactiveEntryLifetime"`
	// PurgeInterval defines the interval at which the throttler checks for
	// expired clients/services.
	PurgeInterval time.Duration `yaml:"purgeInterval"`
	// ConfigRefreshInterval defines the interval at which the throttler requests
	// updated configurations from the ThrottlingService.
	ConfigRefreshInterval time.Duration `yaml:"configRefreshInterval"`
	// ConfigRefreshJitter defines the maximum amount of time to
	// increase from the above ConfigRefreshInterval to avoid multiple
	// agents hitting one ThrottlingService at once.
	ConfigRefreshJitter time.Duration `yaml:"configRefreshJitter"`
}
