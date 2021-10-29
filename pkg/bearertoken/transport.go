package bearertoken

import (
	"errors"
	"net/http"
)

// transport implements the http.RoundTripper interface,
// itself wrapping an instance of http.RoundTripper.
type transport struct {
	defaultToken         string
	allowOverrideFromCtx bool
	wrapped              http.RoundTripper
}

// Option sets attributes of this transport.
type Option func(*transport)

// WithAllowOverrideFromCtx sets whether the defaultToken can be overridden
// with the token within the request context.
func WithAllowOverrideFromCtx(allow bool) Option {
	return func(t *transport) {
		t.allowOverrideFromCtx = allow
	}
}

// WithToken sets the defaultToken that will be injected into the outbound HTTP
// request's Authorization bearer token header.
// If the WithAllowOverrideFromCtx(true) option is provided, the request context's
// bearer token, will be used in preference to this token.
func WithToken(token string) Option {
	return func(t *transport) {
		t.defaultToken = token
	}
}

// NewTransport returns a new bearer token transport that wraps the given
// http.RoundTripper, forwarding the authorization token from inbound to
// outbound HTTP requests.
func NewTransport(roundTripper http.RoundTripper, opts ...Option) http.RoundTripper {
	t := &transport{wrapped: roundTripper}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// RoundTrip injects the outbound Authorization header with the
// token provided in the inbound request.
func (tr *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if tr.wrapped == nil {
		return nil, errors.New("no http.RoundTripper provided")
	}
	token := tr.defaultToken
	if tr.allowOverrideFromCtx {
		headerToken, _ := GetBearerToken(r.Context())
		if headerToken != "" {
			token = headerToken
		}
	}
	r.Header.Set("Authorization", "Bearer "+token)
	return tr.wrapped.RoundTrip(r)
}
