// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ilm

import (
	"context"
	"embed"

	"github.com/jaegertracing/jaeger/pkg/es"
)

//go:embed *.json
var ILM embed.FS

func CreatePolicyIfNotExists(client es.Client, isOpenSearch bool, version uint) error {
	if version < ilmVersionSupport {
		return ErrIlmNotSupported
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !isOpenSearch {
		policyExists, err := client.IlmPolicyExists(ctx, DefaultIlmPolicy)
		if err != nil {
			return err
		}
		if !policyExists {
			policy := loadPolicy(ilmPolicyFile)
			_, err = client.CreateIlmPolicy().Policy(DefaultIlmPolicy).BodyString(policy).Do(ctx)
			if err != nil {
				return err
			}
		}
	} else {
		policyExists, err := client.IsmPolicyExists(ctx, DefaultIsmPolicy)
		if err != nil {
			return err
		}
		if !policyExists {
			policy := loadPolicy(ismPolicyFile)
			_, err = client.CreateIsmPolicy(ctx, DefaultIsmPolicy, policy)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func loadPolicy(name string) string {
	file, _ := ILM.ReadFile(name)
	return string(file)
}
