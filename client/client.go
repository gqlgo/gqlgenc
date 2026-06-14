package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type Client struct {
	client     *http.Client
	endpoint   string
	wsEndpoint string
}

// NewClient creates a new http client wrapper.
//
// The WebSocket endpoint for subscriptions defaults to endpoint with the
// http(s) scheme replaced by ws(s). Use WithWebSocketEndpoint to override it.
// Customize the HTTP behaviour (headers, auth, logging, test transports) by
// wrapping the transport with WithRoundTripper.
func NewClient(endpoint string, options ...Option) *Client {
	client := &Client{
		endpoint:   endpoint,
		wsEndpoint: deriveWebSocketEndpoint(endpoint),
		client:     http.DefaultClient,
	}
	for _, option := range options {
		option(client)
	}

	return client
}

type Option func(*Client)

// WithRoundTripper wraps the HTTP transport used by the client with wrap. The
// wrapper applies to every request, including the WebSocket handshake (which
// goes through the same transport), and composes on top of the existing
// transport so connection pooling is preserved. Use NewHeaderTransport to add
// headers.
//
// Options apply per call, so passing WithRoundTripper to Post, Get or
// Subscribe affects only that call and never mutates the base client or its
// shared http.Client.
func WithRoundTripper(wrap func(http.RoundTripper) http.RoundTripper) Option {
	return func(c *Client) {
		base := c.client.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		// copy the http.Client so wrapping never mutates a shared client
		// (e.g. http.DefaultClient) or other in-flight calls
		cl := *c.client
		cl.Transport = wrap(base)
		c.client = &cl
	}
}

// WithWebSocketEndpoint overrides the endpoint used for subscriptions.
func WithWebSocketEndpoint(endpoint string) Option {
	return func(c *Client) {
		c.wsEndpoint = endpoint
	}
}

// NewHeaderTransport returns a transport wrapper that adds the headers returned
// by header(ctx) to each request. Pass it to WithRoundTripper.
//
// header is evaluated per request, so it supports dynamic values such as a
// rotating auth token or a header derived from the request context. Existing
// header keys are overwritten.
func NewHeaderTransport(header func(ctx context.Context) http.Header) func(http.RoundTripper) http.RoundTripper {
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

// deriveWebSocketEndpoint maps an http(s) endpoint to its ws(s) equivalent.
func deriveWebSocketEndpoint(endpoint string) string {
	if rest, ok := strings.CutPrefix(endpoint, "https://"); ok {
		return "wss://" + rest
	}
	if rest, ok := strings.CutPrefix(endpoint, "http://"); ok {
		return "ws://" + rest
	}

	return endpoint
}

// withOptions returns a shallow copy of c with the per-call options applied.
// It never mutates c, so options passed to Post, Get or Subscribe affect only
// that single call.
func (c *Client) withOptions(options ...Option) *Client {
	cc := *c
	for _, option := range options {
		option(&cc)
	}

	return &cc
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	return ParseResponse(resp, out)
}
