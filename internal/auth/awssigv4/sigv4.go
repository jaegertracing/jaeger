// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package awssigv4

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

const (
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

type RoundTripper struct {
	Transport http.RoundTripper
	Signer    *v4.Signer
	Region    string
	Service   string
	CredsProvider aws.CredentialsProvider
}

func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.Transport == nil {
		return nil, fmt.Errorf("no http.RoundTripper provided")
	}

	creds, err := rt.CredsProvider.Retrieve(req.Context())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	var bodyHash string
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		hash := sha256.Sum256(bodyBytes)
		bodyHash = hex.EncodeToString(hash[:])
	} else {
		bodyHash = emptyPayloadHash
	}

	err = rt.Signer.SignHTTP(
		req.Context(),
		creds,
		req,
		bodyHash,
		rt.Service,
		rt.Region,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	return rt.Transport.RoundTrip(req)
}

func NewRoundTripper(
	transport http.RoundTripper,
	region string,
	service string,
	accessKeyID string,
	secretAccessKey string,
	sessionToken string,
	profile string,
) (*RoundTripper, error) {
	if transport == nil {
		transport = http.DefaultTransport
	}

	if region == "" {
		return nil, fmt.Errorf("AWS region is required for SigV4 authentication")
	}

	if service == "" {
		service = "es"
	}

	ctx := context.Background()
	var cfg aws.Config
	var err error

	if accessKeyID != "" && secretAccessKey != "" {
		// use static credentials if provided
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				accessKeyID,
				secretAccessKey,
				sessionToken,
			)),
		)
	} else if profile != "" {
		// use specific profile from shared config
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithSharedConfigProfile(profile),
		)
	} else {
		// use default credential chain
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &RoundTripper{
		Transport:     transport,
		Signer:        v4.NewSigner(),
		Region:        region,
		Service:       service,
		CredsProvider: cfg.Credentials,
	}, nil
}

