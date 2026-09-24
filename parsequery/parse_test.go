package parsequery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestParseQueryDocuments(t *testing.T) {
	t.Parallel()

	schema := gqlparser.MustLoadSchema(&ast.Source{Name: "schema.graphql", Input: `
type Query {
	user(id: ID!): User
}

type User {
	id: ID!
	name: String!
}
`})

	tests := []struct {
		name           string
		sources        []*ast.Source
		wantOperations []string
		wantFragments  []string
		wantErr        string
	}{
		{
			name: "operations and fragments from several sources are merged",
			sources: []*ast.Source{
				{Name: "user.graphql", Input: `query GetUser($id: ID!) { user(id: $id) { ...UserFields } }`},
				{Name: "fragments.graphql", Input: `fragment UserFields on User { id name }`},
			},
			wantOperations: []string{"GetUser"},
			wantFragments:  []string{"UserFields"},
		},
		{
			name:    "syntax error is reported",
			sources: []*ast.Source{{Name: "broken.graphql", Input: `query GetUser { user(id: `}},
			wantErr: "broken.graphql",
		},
		{
			name:    "unknown field fails validation",
			sources: []*ast.Source{{Name: "bad.graphql", Input: `query GetUser { user(id: "1") { email } }`}},
			wantErr: "email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := ParseQueryDocuments(schema, tt.sources)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			operations := make([]string, 0, len(doc.Operations))
			for _, operation := range doc.Operations {
				operations = append(operations, operation.Name)
			}

			fragments := make([]string, 0, len(doc.Fragments))
			for _, fragment := range doc.Fragments {
				fragments = append(fragments, fragment.Name)
			}

			require.Equal(t, tt.wantOperations, operations)
			require.Equal(t, tt.wantFragments, fragments)
		})
	}
}
