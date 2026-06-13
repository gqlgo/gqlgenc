package client

import (
	"fmt"
	"net/http"
)

type Client struct {
	client   *http.Client
	header   http.Header
	endpoint string
}

// NewClient creates a new http client wrapper.
func NewClient(endpoint string, options ...Option) *Client {
	client := &Client{
		endpoint: endpoint,
		client:   http.DefaultClient,
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
