package queryparser

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

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
	setMatrix(matrix: MatrixInput!): Boolean
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

input MatrixInput {
	rows: [[CellInput!]!]
}

input CellInput {
	color: CellColor
}

enum CellColor {
	RED
	GREEN
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
		{
			// 入れ子リスト [[CellInput!]!] の要素型 (CellInput とその enum CellColor) も
			// 再帰的に収集する。1 段だけ剥がす実装だと要素型が漏れて生成されない。
			name: "入れ子リストのInput型の要素型が再帰的に収集される",
			args: args{
				query: `
					mutation SetMatrix($matrix: MatrixInput!) {
						setMatrix(matrix: $matrix)
					}
				`,
			},
			want: want{
				usedTypes: map[string]bool{
					"MatrixInput": true,
					"CellInput":   true,
					"CellColor":   true,
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

const interfaceSchema = `
type Query {
	node: Node
	todos: [Todo!]!
}

interface Node {
	id: ID!
}

type User implements Node {
	id: ID!
	name: String!
}

type Post implements Node {
	id: ID!
	title: String!
}

type Todo {
	id: ID!
	text: String!
}
`

// collectFieldNames は選択セット直下の Field 名を順に返す。
func collectFieldNames(selectionSet ast.SelectionSet) []string {
	var names []string
	for _, selection := range selectionSet {
		if field, ok := selection.(*ast.Field); ok {
			names = append(names, field.Name)
		}
	}
	return names
}

func TestInjectTypenames(t *testing.T) {
	t.Parallel()

	type args struct {
		query string
	}

	type want struct {
		nodeFieldNames []string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// インラインフラグメントがあり __typename が無い → 先頭に __typename を注入する
			name: "インラインフラグメントに__typenameが自動注入される",
			args: args{
				query: `
					query GetNode {
						node {
							id
							... on User { name }
							... on Post { title }
						}
					}
				`,
			},
			want: want{
				nodeFieldNames: []string{"__typename", "id"},
			},
		},
		{
			// 既に __typename がある → 重複させない
			name: "既に__typenameがある場合は重複しない",
			args: args{
				query: `
					query GetNode {
						node {
							__typename
							id
							... on User { name }
						}
					}
				`,
			},
			want: want{
				nodeFieldNames: []string{"__typename", "id"},
			},
		},
		{
			// インラインフラグメントが無い → __typename を注入しない
			name: "インラインフラグメントが無い場合は注入しない",
			args: args{
				query: `
					query GetNode {
						node {
							id
						}
					}
				`,
			},
			want: want{
				nodeFieldNames: []string{"id"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := gqlparser.MustLoadSchema(&ast.Source{Input: interfaceSchema})
			queryDocument, err := QueryDocument(schema, []*ast.Source{{Input: tt.args.query}})
			if err != nil {
				t.Fatalf("QueryDocument() error = %v", err)
			}

			// node フィールドの選択セットを取り出す
			var nodeSelectionSet ast.SelectionSet
			for _, selection := range queryDocument.Operations[0].SelectionSet {
				if field, ok := selection.(*ast.Field); ok && field.Name == "node" {
					nodeSelectionSet = field.SelectionSet
				}
			}

			got := collectFieldNames(nodeSelectionSet)
			if diff := cmp.Diff(tt.want.nodeFieldNames, got); diff != "" {
				t.Errorf("node field names diff(-want +got): %s", diff)
			}
		})
	}
}

func TestInjectGoFragmentDirective(t *testing.T) {
	t.Parallel()

	type want struct {
		hasFragmentDefinition bool
		hasField              bool
	}

	tests := []struct {
		name   string
		schema string
		want   want
	}{
		{
			// スキーマに gqlgen の @goModel があっても @goFragment を別途注入する
			name: "既存の@goModel定義があっても@goFragmentを注入する",
			schema: `
				directive @goModel(model: String) on OBJECT | INPUT_OBJECT
				type Query { node: Node }
				interface Node { id: ID! }
				type User implements Node { id: ID! name: String! }
			`,
			want: want{
				hasFragmentDefinition: true,
				hasField:              true,
			},
		},
		{
			// @goFragment が未宣言 → 定義ごと注入する
			name: "@goFragmentが未宣言なら定義を注入する",
			schema: `
				type Query { node: Node }
				interface Node { id: ID! }
				type User implements Node { id: ID! name: String! }
			`,
			want: want{
				hasFragmentDefinition: true,
				hasField:              true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := gqlparser.MustLoadSchema(&ast.Source{Input: tt.schema})
			injectGoFragmentDirective(schema)

			directive := schema.Directives[goFragmentDirectiveName]
			if directive == nil {
				t.Fatal("goFragment directive is nil")
			}

			gotFragmentDefinition := slices.Contains(directive.Locations, ast.LocationFragmentDefinition)
			if gotFragmentDefinition != tt.want.hasFragmentDefinition {
				t.Errorf("FRAGMENT_DEFINITION location = %v, want %v", gotFragmentDefinition, tt.want.hasFragmentDefinition)
			}

			gotField := slices.Contains(directive.Locations, ast.LocationField)
			if gotField != tt.want.hasField {
				t.Errorf("FIELD location = %v, want %v", gotField, tt.want.hasField)
			}
		})
	}
}

func TestQueryDocument_DuplicateGoOperationName(t *testing.T) {
	t.Parallel()

	type args struct {
		query string
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// getTodos と GetTodos はどちらも Go 名が GetTodos になり衝突する → エラー
			name: "Go名が衝突するオペレーションはエラー",
			args: args{
				query: `
					query getTodos { todos { id } }
					query GetTodos { todos { id } }
				`,
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
		{
			// Go 名が衝突しなければエラーにならない
			name: "Go名が衝突しなければエラーにならない",
			args: args{
				query: `
					query GetTodos { todos { id } }
					query GetTodosBySortOrder { todosBySortOrder(order: ASC) { id } }
				`,
			},
			want: want{
				err: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := gqlparser.MustLoadSchema(&ast.Source{Input: testSchema})
			_, err := QueryDocument(schema, []*ast.Source{{Input: tt.args.query}})

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}
		})
	}
}
