package config

import (
	"context"
	"fmt"
	"maps"
	"net/http"
)

type HeaderTransport struct {
	base   http.RoundTripper
	header func(ctx context.Context) http.Header
}

func NewHeaderTransport(header func(ctx context.Context) http.Header) func(http.RoundTripper) http.RoundTripper {
	return func(base http.RoundTripper) http.RoundTripper {
		return &HeaderTransport{
			base:   base,
			header: header,
		}
	}
}

func (t *HeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	req = req.Clone(ctx)

	header := t.header(ctx)
	maps.Copy(req.Header, header)

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf(": %w", err)
	}

	return resp, nil
}

func TransportAppend(roundTripper http.RoundTripper, newRoundTrippers ...func(http.RoundTripper) http.RoundTripper) http.RoundTripper {
	for _, newRoundTripper := range newRoundTrippers {
		roundTripper = newRoundTripper(roundTripper)
	}

	return roundTripper
}
