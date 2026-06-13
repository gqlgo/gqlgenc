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
// Options apply only to this call and do not mutate c.
//
// Even when an error is returned, the result may contain partial data:
// GraphQL servers can return both "data" and "errors" in one response.
//
// Variables containing graphql.Upload values are sent as a
// multipart/form-data request following the GraphQL multipart request spec.
func Do[Vars, Res any](ctx context.Context, c *Client, op Operation[Vars, Res], vars Vars, options ...Option) (*Res, error) {
	cc := *c
	for _, option := range options {
		option(&cc)
	}

	req, err := NewRequest(ctx, cc.endpoint, op.Name, op.Document, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to create post request: %w", err)
	}

	var res Res
	err = cc.do(req, &res)

	return &res, err
}
