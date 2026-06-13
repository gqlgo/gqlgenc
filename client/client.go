package client

import (
	"fmt"
	"net/http"
	"strings"
)

type Client struct {
	client     *http.Client
	header     http.Header
	endpoint   string
	wsEndpoint string
}

// NewClient creates a new http client wrapper.
//
// The WebSocket endpoint for subscriptions defaults to endpoint with the
// http(s) scheme replaced by ws(s). Use WithWebSocketEndpoint to override it.
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

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.client = httpClient
	}
}

func WithHTTPHeader(header http.Header) Option {
	return func(c *Client) {
		c.header = header
	}
}

// WithWebSocketEndpoint overrides the endpoint used for subscriptions.
func WithWebSocketEndpoint(endpoint string) Option {
	return func(c *Client) {
		c.wsEndpoint = endpoint
	}
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

func (c *Client) do(req *http.Request, out any) error {
	for key, values := range c.header {
		req.Header[http.CanonicalHeaderKey(key)] = values
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	return ParseResponse(resp, out)
}
