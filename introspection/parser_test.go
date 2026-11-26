package introspection

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIntrospectionQuery_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filename    string
		expectedErr error
	}{
		{"no mutation in schema", "testdata/introspection_result_no_mutation.json", nil},
	}

	for _, testCase := range tests {
		test := testCase
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			query := readQueryResult(t, test.filename)

			if test.expectedErr == nil {
				require.NotPanics(t, func() {
					ast := ParseIntrospectionQuery("test", query)
					require.NotNil(t, ast)
				})
			} else {
				require.PanicsWithValue(t, test.expectedErr, func() {
					ast := ParseIntrospectionQuery("test", query)
					require.Nil(t, ast)
				})
			}
		})
	}
}

func readQueryResult(t *testing.T, filename string) Query {
	t.Helper()

	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	query := Query{}
	err = json.Unmarshal(data, &query)
	require.NoError(t, err)

	return query
}

func TestParseIntrospectionQuery_deprecatedInputsAndRepeatableDirectives(t *testing.T) {
	t.Parallel()

	typeName := func(name string) *string { return &name }
	reason := "use new"
	booleanType := TypeRef{Kind: TypeKindScalar, Name: typeName("Boolean")}
	deprecatedInput := &InputValue{Name: "old", Type: booleanType, IsDeprecated: true, DeprecationReason: &reason}
	currentInput := &InputValue{Name: "new", Type: booleanType}

	var query Query

	query.Schema.QueryType.Name = typeName("Query")
	query.Schema.Types = FullTypes{
		{Kind: TypeKindScalar, Name: typeName("Boolean")},
		{Kind: TypeKindInputObject, Name: typeName("Filter"), InputFields: []*InputValue{deprecatedInput, currentInput}},
		{Kind: TypeKindObject, Name: typeName("Query"), Fields: []*FieldValue{
			{Name: "test", Type: booleanType, Args: []*InputValue{deprecatedInput, currentInput}},
		}},
	}
	query.Schema.Directives = []*DirectiveType{
		{Name: "tag", Locations: []string{"FIELD"}, IsRepeatable: true},
		{Name: "once", Locations: []string{"FIELD"}},
	}

	doc := ParseIntrospectionQuery("test", query)

	require.True(t, doc.Directives.ForName("tag").IsRepeatable)
	require.False(t, doc.Directives.ForName("once").IsRepeatable)

	arguments := doc.Definitions.ForName("Query").Fields.ForName("test").Arguments
	deprecatedArgument := arguments.ForName("old").Directives.ForName("deprecated")
	require.NotNil(t, deprecatedArgument)
	require.Equal(t, "use new", deprecatedArgument.Arguments.ForName("reason").Value.Raw)
	require.Empty(t, arguments.ForName("new").Directives)

	inputFields := doc.Definitions.ForName("Filter").Fields
	deprecatedInputField := inputFields.ForName("old").Directives.ForName("deprecated")
	require.NotNil(t, deprecatedInputField)
	require.Equal(t, "use new", deprecatedInputField.Arguments.ForName("reason").Value.Raw)
	require.Empty(t, inputFields.ForName("new").Directives)
}
