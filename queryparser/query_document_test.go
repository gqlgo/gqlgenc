package queryparser

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
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

func TestTypesFromQueryDocuments(t *testing.T) {
	t.Parallel()

	type args struct {
		query string
	}

	type want struct {
		usedTypes map[string]bool
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// レスポンスのセレクションセットで参照されたenumを収集する
			name: "レスポンスフィールドで使用されているenumが収集される",
			args: args{
				query: `
					mutation CreateMany($todos: NewTodos!) {
						createTodos(input: $todos) {
							todos { id status }
						}
					}
				`,
			},
			want: want{
				usedTypes: map[string]bool{
					"NewTodos":   true,
					"NewTodo":    true,
					"String":     true,
					"TodoStatus": true,
				},
			},
		},
		{
			// オペレーション引数の変数定義からenumを収集する
			name: "オペレーション引数としてのみ使用されているenumが収集される",
			args: args{
				query: `
					query GetBySortOrder($order: SortOrder!) {
						todosBySortOrder(order: $order) { id }
					}
				`,
			},
			want: want{
				usedTypes: map[string]bool{
					"SortOrder": true,
				},
			},
		},
		{
			// 変数定義のInput型はネストしたInput型も再帰的に収集する
			name: "変数定義のInput型が再帰的に収集される",
			args: args{
				query: `
					mutation CreateMany($todos: NewTodos!) {
						createTodos(input: $todos) {
							todos { id }
						}
					}
				`,
			},
			want: want{
				usedTypes: map[string]bool{
					"NewTodos": true,
					"NewTodo":  true,
					"String":   true,
				},
			},
		},
		{
			// フラグメント内のセレクションセットも辿ってenumを収集する
			name: "フラグメントスプレッド内で使用されているenumが収集される",
			args: args{
				query: `
					fragment TodoFields on Todo {
						id
						status
					}

					query GetTodos {
						todos {
							...TodoFields
						}
					}
				`,
			},
			want: want{
				usedTypes: map[string]bool{
					"TodoStatus": true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			schema := gqlparser.MustLoadSchema(&ast.Source{Input: testSchema})
			queryDocument, err := QueryDocument(schema, []*ast.Source{{Input: tt.args.query}})
			if err != nil {
				t.Fatalf("QueryDocument() error = %v", err)
			}
			queryDocuments, err := OperationQueryDocuments(schema, queryDocument.Operations)
			if err != nil {
				t.Fatalf("OperationQueryDocuments() error = %v", err)
			}

			got := TypesFromQueryDocuments(schema, queryDocuments)

			if diff := cmp.Diff(tt.want.usedTypes, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
