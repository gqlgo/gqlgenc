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
// gqlgenc does not own http.Client configuration: there are no built-in helper
// options. Customize the HTTP behaviour (headers, auth, timeouts, logging, test
// transports) by writing your own Option that returns the http.Client to use.
// See the README for the rationale and examples.
func NewClient(endpoint string, options ...Option) *Client {
	return &Client{
		client:   applyOptions(http.DefaultClient, options),
		endpoint: endpoint,
	}
}

// Option configures the http.Client a client uses. It returns the client to
// use, which may be a copy, so an Option never mutates a shared http.Client.
// Options apply per call when passed to Post, Get or Subscribe, affecting only
// that call. Callers provide their own Options; gqlgenc ships none.
type Option func(*http.Client) *http.Client

func applyOptions(httpClient *http.Client, options []Option) *http.Client {
	for _, option := range options {
		httpClient = option(httpClient)
	}

	return httpClient
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

	return parseResponse(resp, out)
}
