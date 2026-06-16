// Package clientopt holds http.Client options used internally by gqlgenc and its
// tests. They are deliberately not part of the public client API: gqlgenc does
// not own http.Client configuration and gives full control to the caller, so
// library users write their own client.Option (see the README). gqlgenc's own
// code (e.g. introspection header injection) and tests use these helpers.
//
// The functions return the bare func(*http.Client) *http.Client signature, which
// is assignable to client.Option, so this package does not import client and can
// be used from the client package's own tests without an import cycle.
package clientopt

import "net/http"

// WithRoundTripper returns an option that wraps the client's transport with wrap.
// It copies the http.Client so a shared client (e.g. http.DefaultClient) is never
// mutated, and falls back to http.DefaultTransport when no transport is set.
func WithRoundTripper(wrap func(http.RoundTripper) http.RoundTripper) func(*http.Client) *http.Client {
	return func(httpClient *http.Client) *http.Client {
		base := httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		// copy the http.Client so wrapping never mutates a shared client or other
		// in-flight calls
		cl := *httpClient
		cl.Transport = wrap(base)

		return &cl
	}
}
