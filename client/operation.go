package client

import (
	"context"
	"fmt"
)

// Operation is a typed GraphQL operation that pairs a query document with
// its variables type Vars and response type Res.
type Operation[Vars, Res any] struct {
	Name     string
	Document string
}

// Do executes op against c with typed variables and returns the decoded
// response. Vars is marshaled as the "variables" member of the request,
// so unlike a map every variable is checked at compile time.
//
// Do does not build multipart requests: operations whose variables contain
// graphql.Upload must use Post or the generated client methods instead.
func Do[Vars, Res any](ctx context.Context, c *Client, op Operation[Vars, Res], vars Vars, options ...Option) (*Res, error) {
	for _, option := range options {
		option(c)
	}

	req, err := NewRequest(ctx, c.endpoint, op.Name, op.Document, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to create post request: %w", err)
	}

	var res Res
	if err := c.do(req, &res); err != nil {
		return nil, err
	}

	return &res, nil
}
