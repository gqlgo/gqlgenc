package querydocument_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/gqlgo/gqlgenc/querydocument"
)

const testSchema = `
type Query {
	todos: [Todo!]!
	todosBySortOrder(order: SortOrder!): [Todo!]!
}

type Mutation {
	createTodos(input: NewTodos!): TodoPage
}

type Todo {
	id: ID!
	text: String!
	status: TodoStatus!
	unusedField: UnusedEnum
}

type TodoPage {
	todos: [Todo!]!
}

input NewTodo {
	text: String!
	userId: String!
}

input NewTodos {
	todos: [NewTodo!]!
}

enum TodoStatus {
	OPEN
	DONE
}

enum SortOrder {
	ASC
	DESC
}

enum UnusedEnum {
	FOO
	BAR
}
`

func loadSchemaAndQuery(t *testing.T, query string) (*ast.Schema, []*ast.QueryDocument) {
	t.Helper()

	schema := gqlparser.MustLoadSchema(&ast.Source{Input: testSchema})
	doc, errs := gqlparser.LoadQuery(schema, query)
	require.Empty(t, errs)

	docs, err := querydocument.QueryDocumentsByOperations(schema, doc.Operations)
	require.NoError(t, err)

	return schema, docs
}

func TestCollectTypesFromQueryDocuments(t *testing.T) {
	t.Parallel()

	t.Run("enum in response field", func(t *testing.T) {
		t.Parallel()

		schema, docs := loadSchemaAndQuery(t, `
			mutation CreateMany($todos: NewTodos!) {
				createTodos(input: $todos) {
					todos { id status }
				}
			}
		`)

		usedTypes := querydocument.CollectTypesFromQueryDocuments(schema, docs)

		require.True(t, usedTypes["TodoStatus"], "enum selected in response should be collected")
		require.False(t, usedTypes["UnusedEnum"], "enum not selected in any query should not be collected")
		require.False(t, usedTypes["SortOrder"], "enum not referenced in this query should not be collected")
	})

	t.Run("enum only as argument", func(t *testing.T) {
		t.Parallel()

		schema, docs := loadSchemaAndQuery(t, `
			query GetBySortOrder($order: SortOrder!) {
				todosBySortOrder(order: $order) { id }
			}
		`)

		usedTypes := querydocument.CollectTypesFromQueryDocuments(schema, docs)

		require.True(t, usedTypes["SortOrder"], "enum used only as operation argument should be collected")
		require.False(t, usedTypes["TodoStatus"], "enum not referenced in this query should not be collected")
		require.False(t, usedTypes["UnusedEnum"], "unreferenced enum should not be collected")
	})

	t.Run("input types from variables", func(t *testing.T) {
		t.Parallel()

		schema, docs := loadSchemaAndQuery(t, `
			mutation CreateMany($todos: NewTodos!) {
				createTodos(input: $todos) {
					todos { id }
				}
			}
		`)

		usedTypes := querydocument.CollectTypesFromQueryDocuments(schema, docs)

		require.True(t, usedTypes["NewTodos"], "input type from variable definition should be collected")
		require.True(t, usedTypes["NewTodo"], "nested input type should be collected recursively")
		require.False(t, usedTypes["TodoStatus"], "enum not selected in response should not be collected")
		require.False(t, usedTypes["UnusedEnum"], "unreferenced enum should not be collected")
	})

	t.Run("enum in fragment spread", func(t *testing.T) {
		t.Parallel()

		schema, docs := loadSchemaAndQuery(t, `
			fragment TodoFields on Todo {
				id
				status
			}

			query GetTodos {
				todos {
					...TodoFields
				}
			}
		`)

		usedTypes := querydocument.CollectTypesFromQueryDocuments(schema, docs)

		require.True(t, usedTypes["TodoStatus"], "enum selected inside a fragment spread should be collected")
		require.False(t, usedTypes["UnusedEnum"], "unreferenced enum should not be collected")
	})

}
