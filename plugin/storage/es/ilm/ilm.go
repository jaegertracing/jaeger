// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ilm

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/es/filter"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
)

//go:embed *.json
var ILM embed.FS

const (
	defaultIlmPolicy    = "jaeger-default-ilm-policy"
	defaultIsmPolicy    = "jaeger-default-ism-policy"
	writeAliasFormat    = "%s-write"
	readAliasFormat     = "%s-read"
	rolloverIndexFormat = "%s-000001"
	ilmVersionSupport   = 7
	ilmPolicyFile       = "ilm-policy.json"
	ismPolicyFile       = "ism-policy.json"
)

type ApplyOn int

const (
	OnArchive ApplyOn = iota
	OnDependency
	OnServiceAndSpan
	OnSampling
)

var ErrIlmNotSupported = errors.New("ILM is supported only for ES version 7+")

type PolicyManager struct {
	client  es.Client
	config  *config.Configuration
	applyOn ApplyOn
}

func (p *PolicyManager) Init() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if p.config.Version < ilmVersionSupport {
		return ErrIlmNotSupported
	}
	err := p.createPolicyIfNotExists(ctx)
	if err != nil {
		return err
	}
	rolloverIndices := p.rolloverIndices()
	for _, index := range rolloverIndices {
		err = p.init(ctx, index)
		if err != nil {
			return err
		}
	}
	return nil
}

func NewPolicyManager(cl es.Client, configuration *config.Configuration, applyOn ApplyOn) *PolicyManager {
	return &PolicyManager{
		client:  cl,
		config:  configuration,
		applyOn: applyOn,
	}
}

func (p *PolicyManager) createPolicyIfNotExists(ctx context.Context) error {
	if p.config.IsOpenSearch {
		policyExists, err := p.client.IsmPolicyExists(ctx, defaultIsmPolicy)
		if err != nil {
			return err
		}
		if !policyExists {
			policy := loadPolicy(ismPolicyFile)
			_, err := p.client.CreateIsmPolicy(ctx, defaultIsmPolicy, policy)
			if err != nil {
				return err
			}
		}
	} else {
		policyExists, err := p.client.IlmPolicyExists(ctx, defaultIlmPolicy)
		if err != nil {
			return err
		}
		if !policyExists {
			policy := loadPolicy(ilmPolicyFile)
			_, err := p.client.CreateIlmPolicy().Policy(defaultIlmPolicy).BodyString(policy).Do(ctx)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *PolicyManager) init(ctx context.Context, indexOpt indexOption) error {
	if p.config.CreateIndexTemplates {
		mappingType, err := mappings.MappingTypeFromString(indexOpt.mapping)
		if err != nil {
			return err
		}
		mapping, err := p.getMapping(mappingType)
		if err != nil {
			return err
		}
		_, err = p.client.CreateTemplate(indexOpt.templateName(p.config.Indices.IndexPrefix)).Body(mapping).Do(ctx)
		if err != nil {
			return err
		}
	}
	index := indexOpt.initialRolloverIndex(p.config.Indices.IndexPrefix)
	err := p.createIndexIfNotExists(ctx, index)
	if err != nil {
		return err
	}
	jaegerIndices, err := p.getJaegerIndices(ctx, indexOpt)
	if err != nil {
		return err
	}
	readAlias := indexOpt.readAliasName(p.config.Indices.IndexPrefix)
	writeAlias := indexOpt.writeAliasName(p.config.Indices.IndexPrefix)
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
			IsWriteIndex: p.config.UseILM,
		})
	}
	for _, alias := range aliases {
		_, err = p.client.CreateAlias(alias.Name).Index(alias.Index).IsWriteIndex(alias.IsWriteIndex).Do(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PolicyManager) getMapping(mappingType mappings.MappingType) (string, error) {
	mappingBuilder := &mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices:         p.config.Indices,
		EsVersion:       p.config.Version,
		UseILM:          p.config.UseILM,
	}
	if p.config.IsOpenSearch {
		mappingBuilder.IsOpenSearch = true
		mappingBuilder.ILMPolicyName = defaultIsmPolicy
	} else {
		mappingBuilder.ILMPolicyName = defaultIlmPolicy
	}
	return mappingBuilder.GetMapping(mappingType)
}

func (p *PolicyManager) createIndexIfNotExists(ctx context.Context, index string) error {
	exists, err := p.client.IndexExists(index).Do(ctx)
	if err != nil {
		return err
	}
	if !exists {
		_, err = p.client.CreateIndex(index).Do(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PolicyManager) getJaegerIndices(ctx context.Context, indexOpt indexOption) ([]client.Index, error) {
	// One of the example of the prefix is: jaeger-main-jaeger-span-* where jaeger-main is the index prefix
	prefix := indexOpt.indexName(p.config.Indices.IndexPrefix) + "-*"
	res, err := p.client.GetIndices().Index(prefix).Do(ctx)
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

type indexOption struct {
	indexType string
	mapping   string
}

func (i *indexOption) indexName(indexPrefix config.IndexPrefix) string {
	return indexPrefix.Apply(i.indexType)
}

// readAliasName returns read alias name of the index
func (i *indexOption) readAliasName(indexPrefix config.IndexPrefix) string {
	return fmt.Sprintf(readAliasFormat, i.indexName(indexPrefix))
}

// writeAliasName returns write alias name of the index
func (i *indexOption) writeAliasName(indexPrefix config.IndexPrefix) string {
	return fmt.Sprintf(writeAliasFormat, i.indexName(indexPrefix))
}

// initialRolloverIndex returns the initial index rollover name
func (i *indexOption) initialRolloverIndex(indexPrefix config.IndexPrefix) string {
	return fmt.Sprintf(rolloverIndexFormat, i.indexName(indexPrefix))
}

// templateName returns the prefixed template name
func (i *indexOption) templateName(indexPrefix config.IndexPrefix) string {
	return indexPrefix.Apply(i.mapping)
}

func (p *PolicyManager) rolloverIndices() []indexOption {
	switch p.applyOn {
	case OnArchive:
		return []indexOption{
			{
				indexType: "jaeger-span-archive",
				mapping:   "jaeger-span",
			},
		}
	case OnDependency:
		return []indexOption{
			{
				mapping:   "jaeger-dependencies",
				indexType: "jaeger-dependencies",
			},
		}
	case OnServiceAndSpan:
		return []indexOption{
			{
				mapping:   "jaeger-span",
				indexType: "jaeger-span",
			},
			{
				mapping:   "jaeger-service",
				indexType: "jaeger-service",
			},
		}
	case OnSampling:
		return []indexOption{
			{
				mapping:   "jaeger-sampling",
				indexType: "jaeger-sampling",
			},
		}
	}
	return []indexOption{}
}

func loadPolicy(name string) string {
	file, _ := ILM.ReadFile(name)
	return string(file)
}
