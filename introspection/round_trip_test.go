package introspection_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gqlgo/gqlgenc/graphqljson"
	"github.com/gqlgo/gqlgenc/introspection"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
	"github.com/vektah/gqlparser/v2/validator"
)

// TestParseIntrospectionQuery_roundTrip loads the introspection response of a
// schema that uses every kind of type and compares the result with the schema
// itself.
//
// testdata/full/response.json is the response of graphql-js 16.15.0 to
// NewQuery with every capability set, for testdata/full/schema.graphql.
func TestParseIntrospectionQuery_roundTrip(t *testing.T) {
	t.Parallel()

	expectedSchema, err := gqlparser.LoadSchema(readSource(t, "testdata/full/schema.graphql"))
	require.NoError(t, err)

	var response struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(readSource(t, "testdata/full/response.json").Input), &response))

	var query introspection.Query
	require.NoError(t, graphqljson.UnmarshalData(response.Data, &query))

	actualSchema, err := validator.ValidateSchemaDocument(introspection.ParseIntrospectionQuery("remote", query))
	require.NoError(t, err)

	require.Equal(t, "Query", actualSchema.Query.Name)
	require.Equal(t, "Mutation", actualSchema.Mutation.Name)
	require.Equal(t, "Subscription", actualSchema.Subscription.Name)

	// A server reports the directives of the specification like its own, while
	// the schema loaded from a file has them as built-ins the formatter omits.
	for directiveName := range gqlparser.MustLoadSchema().Directives {
		delete(actualSchema.Directives, directiveName)
	}

	require.Equal(t, formatSchema(expectedSchema), formatSchema(actualSchema))
}

func readSource(t *testing.T, filename string) *ast.Source {
	t.Helper()

	content, err := os.ReadFile(filename)
	require.NoError(t, err)

	return &ast.Source{Name: filename, Input: string(content)}
}

func formatSchema(schema *ast.Schema) string {
	var buf bytes.Buffer
	formatter.NewFormatter(&buf).FormatSchema(schema)

	return buf.String()
}
