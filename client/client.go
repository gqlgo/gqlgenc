package client

import (
	"fmt"
	"net/http"
)

type Client struct {
	client   *http.Client
	endpoint string
}

// NewClient creates a client for query and mutation operations (Post, Get).
// Subscriptions use a separate client created with NewSubscriptionClient.
//
// Customize the HTTP behaviour (headers, auth, logging, test transports) by
// wrapping the transport with WithRoundTripper.
func NewClient(endpoint string, options ...Option) *Client {
	return &Client{
		client:   applyOptions(http.DefaultClient, options),
		endpoint: endpoint,
	}
}

// Option configures the http.Client a client uses. It returns the client to
// use, which may be a copy, so an Option never mutates a shared http.Client.
type Option func(*http.Client) *http.Client

func applyOptions(httpClient *http.Client, options []Option) *http.Client {
	for _, option := range options {
		httpClient = option(httpClient)
	}

	return httpClient
}

// WithRoundTripper wraps the HTTP transport with wrap. The wrapper applies to
// every request, including the WebSocket handshake of the subscription client,
// and composes on top of the existing transport so connection pooling is
// preserved. Use it to add headers, authentication or logging with your own
// http.RoundTripper.
//
// Options apply per call, so passing WithRoundTripper to Post, Get or Subscribe
// affects only that call and never mutates the base client or a shared
// http.Client.
func WithRoundTripper(wrap func(http.RoundTripper) http.RoundTripper) Option {
	return func(httpClient *http.Client) *http.Client {
		base := httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		// copy the http.Client so wrapping never mutates a shared client
		// (e.g. http.DefaultClient) or other in-flight calls
		cl := *httpClient
		cl.Transport = wrap(base)

		return &cl
	}
}

// withOptions returns a shallow copy of c with the per-call options applied.
// It never mutates c, so options passed to Post or Get affect only that call.
func (c *Client) withOptions(options ...Option) *Client {
	cc := *c
	cc.client = applyOptions(cc.client, options)

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
