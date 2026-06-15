// Package transport provides http.RoundTripper wrappers used by the client.
package transport

import (
	"context"
	"fmt"
	"net/http"
)

// NewHeader returns a RoundTripper wrapper that adds the headers returned by
// header(ctx) to each request. header is evaluated per request, so it supports
// dynamic values such as a rotating auth token or a header derived from the
// request context. Existing header keys are overwritten.
func NewHeader(header func(ctx context.Context) http.Header) func(http.RoundTripper) http.RoundTripper {
	return func(base http.RoundTripper) http.RoundTripper {
		return &headerTransport{base: base, header: header}
	}
}

type headerTransport struct {
	base   http.RoundTripper
	header func(ctx context.Context) http.Header
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	// RoundTrippers must not modify the request, so clone before adding headers
	clone := req.Clone(ctx)
	for key, values := range t.header(ctx) {
		clone.Header[http.CanonicalHeaderKey(key)] = values
	}

	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, fmt.Errorf("header transport: %w", err)
	}

	return resp, nil
}
