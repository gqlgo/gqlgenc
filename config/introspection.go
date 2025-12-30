package config

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/introspection"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

func introspectionSchema(ctx context.Context, httpClient *http.Client, endpoint string, header http.Header) (*ast.Schema, error) {
	//httpClient := http.DefaultClient
	//if c.GQLGencConfig.Endpoint.Client != nil {
	//	httpClient = c.GQLGencConfig.Endpoint.Client
	//}

	gqlgencClient := client.NewClient(endpoint, client.WithHTTPClient(httpClient))

	var res introspection.Query
	if err := gqlgencClient.Post(ctx, "Query", introspection.Introspection, nil, &res); err != nil {
		return nil, fmt.Errorf("introspection query failed: %w", err)
	}

	schema, err := validator.ValidateSchemaDocument(introspection.SchemaFromIntrospection(endpoint, res))
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
