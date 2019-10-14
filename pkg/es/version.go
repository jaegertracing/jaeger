package es

import (
	"fmt"
	"errors"
	"golang.org/x/tools/internal/semver"
)

// Version is a wrapper around semantic-version
type Version struct {
	number string
}

// NewVersion is a simple wrapper around semantic-version
func NewVersion(number string) (*Version, error) {
	v := &Version{}

	v.number = fmt.Sprintf("v%s", number) // Prefix "v"
	if !semver.IsValid(v.number) {
		return nil, errors.New("Invalid elasticsearch version")
	}

	return v, nil
}

func (v *Version) String() string {
	return v.number
}

// IsES7 returns true if version is compatible with ElasticSearch >=7
func (v *Version) IsES7() bool {
	return semver.Major(v.number) == "v7" || semver.MajorMinor(v.number) == "v6.8"
}
