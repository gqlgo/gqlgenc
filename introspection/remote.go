package introspection

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/introspection/internal/transport"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

// introspectionOperation is the typed operation for the GraphQL schema introspection
// query. It takes no variables.
var introspectionOperation = client.Operation[client.Query, struct{}, Query]{
	Name:     "Query",
	Document: Introspection,
}

// LoadRemoteSchema fetches the schema from a GraphQL endpoint via an
// introspection query and returns it as a validated AST. It reuses the runtime
// client so that the config package can stay free of the client dependency:
// the layer that handles config (run.go) calls this and assigns the result.
func LoadRemoteSchema(ctx context.Context, endpoint string, header http.Header) (*ast.Schema, error) {
	var options []client.Option
	if len(header) > 0 {
		options = append(options, client.WithRoundTripper(transport.NewHeader(func(context.Context) http.Header {
			return header
		})))
	}

	gqlClient := client.NewClient(endpoint, options...)

	res, err := gqlClient.Post(ctx, introspectionOperation, struct{}{})
	if err != nil {
		return nil, fmt.Errorf("introspection query failed: %w", err)
	}

	schema, err := validator.ValidateSchemaDocument(SchemaFromIntrospection(endpoint, *res))
	if err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	if schema.Query == nil {
		schema.Query = &ast.Definition{
			Kind: ast.Object,
			Name: "Query",
		}
		schema.Types["Query"] = schema.Query
	}

	return schema, nil
}
