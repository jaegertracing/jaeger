package client

import (
	"bytes"
)

const (
	// RequestHeaderContentType will be attached to every HTTP request
	RequestHeaderContentType = "Content-Type"
	// AllowedContentType is the type of actual payload
	AllowedContentType = "application/x-thrift"
)

// Client posts the payload which is serialized spans to collector
// and handles the response
type Client interface {
	Post(payload *bytes.Buffer) (err error)
}
