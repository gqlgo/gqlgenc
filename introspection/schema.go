package introspection

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/internal/clientopt"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

// Introspection is the introspection query for a server that supports none of
// the optional parts of introspection. It equals NewQuery(Capabilities{}).
const Introspection = `query Query {
      __schema {
        queryType { name }
        mutationType { name }
        subscriptionType { name }
        types {
          ...FullType
        }
        directives {
          name
          description
          locations
          args {
            ...InputValue
          }
        }
      }
    }

    fragment FullType on __Type {
      kind
      name
      description
      fields(includeDeprecated: true) {
        name
        description
        args {
          ...InputValue
        }
        type {
          ...TypeRef
        }
        isDeprecated
        deprecationReason
      }
      inputFields {
        ...InputValue
      }
      interfaces {
        ...TypeRef
      }
      enumValues(includeDeprecated: true) {
        name
        description
        isDeprecated
        deprecationReason
      }
      possibleTypes {
        ...TypeRef
      }
    }

    fragment InputValue on __InputValue {
      name
      description
      type { ...TypeRef }
      defaultValue
    }

    fragment TypeRef on __Type {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
                ofType {
                  kind
                  name
                  ofType {
                    kind
                    name
                  }
                }
              }
            }
          }
        }
      }
    }`

type TypeKind string

const (
	TypeKindScalar      TypeKind = "SCALAR"
	TypeKindObject      TypeKind = "OBJECT"
	TypeKindInterface   TypeKind = "INTERFACE"
	TypeKindUnion       TypeKind = "UNION"
	TypeKindEnum        TypeKind = "ENUM"
	TypeKindInputObject TypeKind = "INPUT_OBJECT"
	TypeKindList        TypeKind = "LIST"
	TypeKindNonNull     TypeKind = "NON_NULL"
)

type FullTypes []*FullType

func (fs FullTypes) NameMap() map[string]*FullType {
	typeMap := make(map[string]*FullType)
	for _, typ := range fs {
		typeMap[*typ.Name] = typ
	}

	return typeMap
}

type FullType struct {
	Kind          TypeKind      `json:"kind"`
	Name          *string       `json:"name"`
	Description   *string       `json:"description"`
	Fields        []*FieldValue `json:"fields"`
	InputFields   []*InputValue `json:"inputFields"`
	Interfaces    []*TypeRef    `json:"interfaces"`
	EnumValues    []*EnumValue  `json:"enumValues"`
	PossibleTypes []*TypeRef    `json:"possibleTypes"`
}

type EnumValue struct {
	Description       *string `json:"description"`
	DeprecationReason *string `json:"deprecationReason"`
	Name              string  `json:"name"`
	IsDeprecated      bool    `json:"isDeprecated"`
}

type FieldValue struct {
	Type              TypeRef       `json:"type"`
	Description       *string       `json:"description"`
	DeprecationReason *string       `json:"deprecationReason"`
	Name              string        `json:"name"`
	Args              []*InputValue `json:"args"`
	IsDeprecated      bool          `json:"isDeprecated"`
}

// InputValue is an argument or an input field. DeprecationReason and
// IsDeprecated are set only when the server has Capabilities.InputValueDeprecation.
type InputValue struct {
	Type              TypeRef `json:"type"`
	Description       *string `json:"description"`
	DefaultValue      *string `json:"defaultValue"`
	DeprecationReason *string `json:"deprecationReason"`
	Name              string  `json:"name"`
	IsDeprecated      bool    `json:"isDeprecated"`
}

type TypeRef struct {
	Name   *string  `json:"name"`
	OfType *TypeRef `json:"ofType"`
	Kind   TypeKind `json:"kind"`
}

type Query struct {
	Schema struct {
		QueryType struct {
			Name *string `json:"name"`
		} `json:"queryType"`
		MutationType *struct {
			Name *string `json:"name"`
		} `json:"mutationType"`
		SubscriptionType *struct {
			Name *string `json:"name"`
		} `json:"subscriptionType"`
		Types      FullTypes        `json:"types"`
		Directives []*DirectiveType `json:"directives"`
	} `json:"__schema"`
}

// DirectiveType is a directive definition. IsRepeatable is set only when the
// server has Capabilities.RepeatableDirective.
type DirectiveType struct {
	Name         string        `json:"name"`
	Description  *string       `json:"description"`
	Locations    []string      `json:"locations"`
	Args         []*InputValue `json:"args"`
	IsRepeatable bool          `json:"isRepeatable"`
}

// introspectionOperation returns the typed operation for the GraphQL schema
// introspection query sent to a server with the given capabilities. It takes
// no variables.
func introspectionOperation(capabilities Capabilities) client.Operation[client.Query, struct{}, Query] {
	return client.Operation[client.Query, struct{}, Query]{
		Name:     "Query",
		Document: NewQuery(capabilities),
	}
}

// LoadRemoteSchema fetches the schema from a GraphQL endpoint via an
// introspection query and returns it as a validated AST. It reuses the runtime
// client so that the config package can stay free of the client dependency:
// the layer that handles config (run.go) calls this and assigns the result.
//
// The schema is loaded in two steps. A capability query first asks which
// optional parts of introspection the server declares (deprecated arguments
// and input fields, repeatable directives), and the introspection query then
// uses only those parts, because a server built before they existed rejects a
// query that uses them. A server that does not answer the capability query
// gets the query without the optional parts (Introspection).
func LoadRemoteSchema(ctx context.Context, endpoint string, header http.Header) (*ast.Schema, error) {
	var options []client.Option
	if len(header) > 0 {
		options = append(options, clientopt.WithRoundTripper(func(base http.RoundTripper) http.RoundTripper {
			return &headerTransport{base: base, header: header}
		}))
	}

	gqlClient := client.NewClient(endpoint, options...)

	res, err := gqlClient.Post(ctx, introspectionOperation(remoteCapabilities(ctx, gqlClient)), struct{}{})
	if err != nil {
		return nil, fmt.Errorf("introspection query failed: %w", err)
	}

	schemaDoc, err := SchemaFromIntrospection(endpoint, *res)
	if err != nil {
		return nil, err
	}

	schema, err := validator.ValidateSchemaDocument(schemaDoc)
	if err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	return schema, nil
}

// headerTransport は、設定ファイルの schema.endpoint.headers を introspection の各リクエストに付ける
// http.RoundTripper。同じキーのヘッダーは上書きする。
type headerTransport struct {
	base   http.RoundTripper
	header http.Header
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// RoundTripper はリクエストを書き換えてはいけないため、複製にヘッダーを足す
	clone := req.Clone(req.Context())
	for key, values := range t.header {
		clone.Header[http.CanonicalHeaderKey(key)] = values
	}

	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, fmt.Errorf("header transport: %w", err)
	}

	return resp, nil
}
