package client

import (
	"context"
	"fmt"
)

// Query, Mutation and Subscription are phantom marker types for the Kind type
// parameter of Operation. They carry no data and only constrain at compile
// time which execution method an operation can be passed to.
type (
	Query        struct{}
	Mutation     struct{}
	Subscription struct{}
)

// httpKind is the set of operation kinds executable over a single HTTP
// request, i.e. everything except subscriptions.
type httpKind interface {
	Query | Mutation
}

// Operation is a typed GraphQL operation. Kind is one of Query, Mutation or
// Subscription; Vars and Res are the variables and response types.
type Operation[Kind, Vars, Res any] struct {
	Name     string
	Document string
}

// Post executes op against c with typed variables and returns the decoded
// response. Vars is marshaled as the "variables" member of the request,
// so unlike a map every variable is checked at compile time. Only query and
// mutation operations may be passed; subscriptions use Subscribe.
// Options apply only to this call and do not mutate c.
//
// Even when an error is returned, the result may contain partial data:
// GraphQL servers can return both "data" and "errors" in one response.
//
// Variables containing graphql.Upload values are sent as a
// multipart/form-data request following the GraphQL multipart request spec.
func (c *Client) Post[Kind httpKind, Vars, Res any](ctx context.Context, op Operation[Kind, Vars, Res], vars Vars, options ...Option) (*Res, error) {
	cc := c.withOptions(options...)

	req, err := NewRequest(ctx, cc.endpoint, op.Name, op.Document, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to create post request: %w", err)
	}

	var res Res
	err = cc.do(req, &res)

	return &res, err
}

// Get executes a query operation as an HTTP GET request, encoding the
// variables into the URL per the GraphQL-over-HTTP specification. Only query
// operations are allowed: the spec forbids GET for mutations, and the Kind
// type parameter enforces this at compile time. Options apply only to this
// call and do not mutate c.
//
// GET requests have no body, so variables containing graphql.Upload values
// are not supported and result in an error; use Post for uploads. Large
// queries or variables may exceed server URL length limits.
func (c *Client) Get[Vars, Res any](ctx context.Context, op Operation[Query, Vars, Res], vars Vars, options ...Option) (*Res, error) {
	cc := c.withOptions(options...)

	req, err := NewGetRequest(ctx, cc.endpoint, op.Name, op.Document, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to create get request: %w", err)
	}

	var res Res
	err = cc.do(req, &res)

	return &res, err
}
