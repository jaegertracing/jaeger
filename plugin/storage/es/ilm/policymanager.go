// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ilm

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/filter"
)

const (
	DefaultIlmPolicy    = "jaeger-default-ilm-policy"
	DefaultIsmPolicy    = "jaeger-default-ism-policy"
	writeAliasSuffix    = "write"
	readAliasSuffix     = "read"
	rolloverIndexSuffix = "000001"
	ilmVersionSupport   = 7
	ilmPolicyFile       = "ilm-policy.json"
	ismPolicyFile       = "ism-policy.json"
)

var ErrIlmNotSupported = errors.New("ILM is supported only for ES version 7+")

type PolicyManager struct {
	client                         func() es.Client
	prefixedIndexNameWithSeparator string
	version                        uint
	isOpenSearch                   bool
}

func (p *PolicyManager) Init() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if p.version < ilmVersionSupport {
		return ErrIlmNotSupported
	}
	err := p.init(ctx)
	if err != nil {
		return err
	}
	return nil
}

func NewPolicyManager(cl func() es.Client, version uint, isOpenSearch bool, prefixedIndexNameWithSeparator string) *PolicyManager {
	return &PolicyManager{
		client:                         cl,
		version:                        version,
		isOpenSearch:                   isOpenSearch,
		prefixedIndexNameWithSeparator: prefixedIndexNameWithSeparator,
	}
}

func (p *PolicyManager) init(ctx context.Context) error {
	index := p.initialRolloverIndex()
	err := p.createIndexIfNotExists(ctx, index)
	if err != nil {
		return err
	}
	jaegerIndices, err := p.getJaegerIndices(ctx)
	if err != nil {
		return err
	}
	readAlias := p.readAliasName()
	writeAlias := p.writeAliasName()
	var aliases []client.Alias
	if !filter.AliasExists(jaegerIndices, readAlias) {
		aliases = append(aliases, client.Alias{
			Index:        index,
			Name:         readAlias,
			IsWriteIndex: false,
		})
	}
	if !filter.AliasExists(jaegerIndices, writeAlias) {
		aliases = append(aliases, client.Alias{
			Index:        index,
			Name:         writeAlias,
			IsWriteIndex: true,
		})
	}
	for _, alias := range aliases {
		_, err = p.client().CreateAlias(alias.Name).Index(alias.Index).IsWriteIndex(alias.IsWriteIndex).Do(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PolicyManager) createIndexIfNotExists(ctx context.Context, index string) error {
	exists, err := p.client().IndexExists(index).Do(ctx)
	if err != nil {
		return err
	}
	if !exists {
		_, err = p.client().CreateIndex(index).Do(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PolicyManager) getJaegerIndices(ctx context.Context) ([]client.Index, error) {
	// One of the example of the prefix is: jaeger-main-jaeger-span-* where jaeger-main is the index prefix
	prefix := p.prefixedIndexNameWithSeparator + "*"
	res, err := p.client().GetIndices().Index(prefix).Do(ctx)
	if err != nil {
		return nil, err
	}
	indices := make([]client.Index, len(res))
	for idx, opts := range res {
		aliases := map[string]bool{}
		for alias := range opts.Aliases {
			aliases[alias] = true
		}
		// ignoring error and ok, ES should return valid date in string format
		creationDateStr, _ := opts.Settings["index.creation_date"].(string)
		creationDate, _ := strconv.ParseInt(creationDateStr, 10, 64)
		indices = append(indices, client.Index{
			Index:        idx,
			CreationTime: time.Unix(0, int64(time.Millisecond)*creationDate),
			Aliases:      aliases,
		})
	}
	return indices, nil
}

// writeAliasName returns write alias name of the index
func (p *PolicyManager) writeAliasName() string {
	return p.prefixedIndexNameWithSeparator + writeAliasSuffix
}

// readAliasName returns read alias name of the index
func (p *PolicyManager) readAliasName() string {
	return p.prefixedIndexNameWithSeparator + readAliasSuffix
}

// initialRolloverIndex returns the initial index rollover name
func (p *PolicyManager) initialRolloverIndex() string {
	return p.prefixedIndexNameWithSeparator + rolloverIndexSuffix
}
