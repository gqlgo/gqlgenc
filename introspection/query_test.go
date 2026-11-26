package introspection

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestNewQuery(t *testing.T) {
	t.Parallel()

	allCapabilities := Capabilities{
		FieldArgumentDeprecation:     true,
		InputFieldDeprecation:        true,
		DirectiveArgumentDeprecation: true,
		InputValueDeprecation:        true,
		RepeatableDirective:          true,
	}

	t.Run("no capabilities gives the Introspection constant", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, Introspection, NewQuery(Capabilities{}))
	})

	t.Run("each capability adds only its own part", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			capabilities Capabilities
			added        string
		}{
			{"field arguments", Capabilities{FieldArgumentDeprecation: true}, "\n        args(includeDeprecated: true) {"},
			{"input fields", Capabilities{InputFieldDeprecation: true}, "\n      inputFields(includeDeprecated: true) {"},
			{"directive arguments", Capabilities{DirectiveArgumentDeprecation: true}, "\n          args(includeDeprecated: true) {"},
			{"input value deprecation", Capabilities{InputValueDeprecation: true}, "      isDeprecated\n      deprecationReason\n    }\n\n    fragment TypeRef"},
			{"repeatable directives", Capabilities{RepeatableDirective: true}, "          isRepeatable\n"},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				query := NewQuery(test.capabilities)

				require.NotContains(t, Introspection, test.added)
				require.Contains(t, query, test.added)

				for _, other := range tests {
					if other.name != test.name {
						require.NotContains(t, query, other.added)
					}
				}
			})
		}
	})

	t.Run("the queries are valid", func(t *testing.T) {
		t.Parallel()

		schema := gqlparser.MustLoadSchema(&ast.Source{Input: "type Query { test: Boolean }"})

		queries := map[string]string{
			"no capabilities":  NewQuery(Capabilities{}),
			"all capabilities": NewQuery(allCapabilities),
			"capability query": CapabilityQueryDocument,
		}
		for name, query := range queries {
			_, errs := gqlparser.LoadQuery(schema, query)
			require.Empty(t, errs, name)
		}
	})
}
