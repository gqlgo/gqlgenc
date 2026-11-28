package codegen

import (
	gotypes "go/types"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/internal/typebind"

	"github.com/vektah/gqlparser/v2"
	graphql "github.com/vektah/gqlparser/v2/ast"
)

func TestFieldTypeName(t *testing.T) {
	t.Parallel()

	type args struct {
		parentTypeName string
		fieldName      string
	}

	type want struct {
		typeName string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 親型名の先頭を大文字にし、フィールド名を Go の公開名に変換して結合する
			name: "親型名とフィールド名から公開型名を生成する",
			args: args{
				parentTypeName: "userOperation",
				fieldName:      "article",
			},
			want: want{
				typeName: "UserOperation_Article",
			},
		},
		{
			// 親型名が空 (インラインフラグメント) でも firstUpper が panic せず "_<Field>" になる
			name: "親型名が空でもfirstUpperが安全に動く",
			args: args{
				parentTypeName: "",
				fieldName:      "article",
			},
			want: want{
				typeName: "_Article",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fieldTypeName(tt.args.parentTypeName, tt.args.fieldName)

			if diff := cmp.Diff(tt.want.typeName, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestGoTypeName(t *testing.T) {
	t.Parallel()

	named := gotypes.NewNamed(gotypes.NewTypeName(0, nil, "Foo", nil), gotypes.NewStruct(nil, nil), nil)

	type args struct {
		typ gotypes.Type
	}

	type want struct {
		name string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// named 型はその型名を返す
			name: "named型は型名を返す",
			args: args{
				typ: named,
			},
			want: want{
				name: "Foo",
			},
		},
		{
			// ポインタ型は要素を辿って型名を返す
			name: "ポインタ型は要素の型名を返す",
			args: args{
				typ: gotypes.NewPointer(named),
			},
			want: want{
				name: "Foo",
			},
		},
		{
			// named でもポインタでもない型は空文字を返す
			name: "namedでもポインタでもない型は空文字を返す",
			args: args{
				typ: gotypes.Typ[gotypes.String],
			},
			want: want{
				name: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := goTypeName(tt.args.typ)

			if diff := cmp.Diff(tt.want.name, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestResolveModelType(t *testing.T) {
	t.Parallel()

	type args struct {
		binder   *typebind.Binder
		models   gqlgenconfig.TypeMap
		typeName string
		nonNull  bool
	}

	type want struct {
		goType string
		err    error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// UC1 (generate.model.file 未指定) で enum/input が未束縛のとき、panic ではなくエラーを返す
			name: "束縛が無いGraphQL型はpanicせずエラーを返す",
			args: args{
				models:   gqlgenconfig.TypeMap{},
				typeName: "Status",
				nonNull:  true,
			},
			want: want{
				goType: "invalid type",
				err:    cmpopts.AnyError,
			},
		},
		{
			// キーは在るが Model が空のときも未束縛として同じくエラーを返す
			name: "Modelが空のGraphQL型はエラーを返す",
			args: args{
				models:   gqlgenconfig.TypeMap{"Status": gqlgenconfig.TypeMapEntry{}},
				typeName: "Status",
				nonNull:  false,
			},
			want: want{
				goType: "invalid type",
				err:    cmpopts.AnyError,
			},
		},
		{
			// 束縛は在るが指す Go 型を binder が解決できないときもエラーを返す
			// (@goModel(model: "誤り") 相当)。パッケージ区切りの無い束縛名なら FindObject が
			// pkgs を参照せず失敗するため、最小構成の binder (nil pkgs) でも安全に到達する。
			name: "束縛先のGo型を解決できないときはエラーを返す",
			args: args{
				binder:   typebind.New(),
				models:   gqlgenconfig.TypeMap{"Status": gqlgenconfig.TypeMapEntry{Model: gqlgenconfig.StringList{"NoPackageSeparator"}}},
				typeName: "Status",
				nonNull:  true,
			},
			want: want{
				goType: "invalid type",
				err:    cmpopts.AnyError,
			},
		},
		{
			// 束縛が解決できるときはその Go 型を返す。nonNull なのでポインタで包まない。
			// map[string]any は FindType の特殊扱いで pkgs を参照せず解決できる。
			name: "束縛が解決できるときはその型を返す",
			args: args{
				binder:   typebind.New(),
				models:   gqlgenconfig.TypeMap{"Meta": gqlgenconfig.TypeMapEntry{Model: gqlgenconfig.StringList{"map[string]any"}}},
				typeName: "Meta",
				nonNull:  true,
			},
			want: want{
				goType: "map[string]any",
			},
		},
		{
			// nullable のときは解決した型をポインタで包む。
			name: "nullableのときは解決した型をポインタで包む",
			args: args{
				binder:   typebind.New(),
				models:   gqlgenconfig.TypeMap{"Meta": gqlgenconfig.TypeMapEntry{Model: gqlgenconfig.StringList{"map[string]any"}}},
				typeName: "Meta",
				nonNull:  false,
			},
			want: want{
				goType: "*map[string]any",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveModelType(tt.args.binder, tt.args.models, tt.args.typeName, tt.args.nonNull)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.goType, got.String()); diff != "" {
				t.Errorf("goType diff(-want +got): %s", diff)
			}
		})
	}
}

// formatSelections は選択セットを 1 行の文字列にする。フィールドは「別名:名前」(別名が名前と同じなら
// 名前だけ)、@skip / @include は名前の後ろ、サブセレクションは波括弧の中に書く。
func formatSelections(selectionSet graphql.SelectionSet) string {
	formatted := make([]string, 0, len(selectionSet))
	for _, selection := range selectionSet {
		var (
			text       strings.Builder
			directives graphql.DirectiveList
			children   graphql.SelectionSet
		)
		switch sel := selection.(type) {
		case *graphql.Field:
			if sel.Alias != sel.Name {
				text.WriteString(sel.Alias + ":")
			}
			text.WriteString(sel.Name)
			directives, children = sel.Directives, sel.SelectionSet
		case *graphql.InlineFragment:
			text.WriteString("...on " + sel.TypeCondition)
			directives, children = sel.Directives, sel.SelectionSet
		case *graphql.FragmentSpread:
			text.WriteString("..." + sel.Name)
			directives = sel.Directives
		}
		for _, directive := range directives {
			text.WriteString("@" + directive.Name)
		}
		if len(children) > 0 {
			text.WriteString("{" + formatSelections(children) + "}")
		}
		formatted = append(formatted, text.String())
	}

	return strings.Join(formatted, " ")
}

func TestMergeSelections(t *testing.T) {
	t.Parallel()

	schema := gqlparser.MustLoadSchema(&graphql.Source{Input: `
		type Query {
			node: Node
		}
		interface Node {
			id: ID!
			name: String!
			payload: Payload
		}
		type Item implements Node {
			id: ID!
			name: String!
			payload: Payload
			title: String!
		}
		type Comment implements Node {
			id: ID!
			name: String!
			payload: Payload
			body: String!
		}
		type Payload {
			success: Boolean!
			message: String
		}
	`})

	type args struct {
		// query の最初のオペレーションが選択する node フィールドの選択セットを、まとめる対象にする。
		// 空文字なら、空の選択セットを対象にする
		query string
	}

	type want struct {
		selections string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 後の選択だけを残すと、先の選択のサブフィールド (success) が生成型から落ちる (gqlgo/gqlgenc#282)
			name: "同じフィールドを2回選択すると、サブセレクションを合わせた1つの選択になる",
			args: args{
				query: `query Q { node { payload { success } payload { message } } }`,
			},
			want: want{
				selections: "payload{success message}",
			},
		},
		{
			// レスポンスのキーは別名なので、別名が違えば別のフィールドとして残す
			name: "別名が違う同じフィールドはまとめない",
			args: args{
				query: `query Q { node { first: payload { success } second: payload { message } } }`,
			},
			want: want{
				selections: "first:payload{success} second:payload{message}",
			},
		},
		{
			// まとめた選択は最初に現れた位置に置き、ほかの選択の順は変えない
			name: "まとめた選択は最初に現れた位置に置く",
			args: args{
				query: `query Q { node { payload { success } name payload { message } } }`,
			},
			want: want{
				selections: "payload{success message} name",
			},
		},
		{
			// どちらの選択も条件付きなら、両方の条件が偽のときにフィールドが返らない
			name: "すべての選択に@skipか@includeが付いていれば、条件付きのままにする",
			args: args{
				query: `query Q($x: Boolean!) { node { name @include(if: $x) name @skip(if: $x) } }`,
			},
			want: want{
				selections: "name@include",
			},
		},
		{
			// 条件の無い選択が 1 つでもあれば、サーバーは必ずそのフィールドを返す
			name: "後の選択に条件が無ければ、条件付きにしない",
			args: args{
				query: `query Q($x: Boolean!) { node { name @include(if: $x) name } }`,
			},
			want: want{
				selections: "name",
			},
		},
		{
			// 条件付きの選択が後にあっても、先の条件の無い選択が優先される
			name: "先の選択に条件が無ければ、条件付きにしない",
			args: args{
				query: `query Q($x: Boolean!) { node { name name @skip(if: $x) } }`,
			},
			want: want{
				selections: "name",
			},
		},
		{
			// 同じフラグメントを 2 回埋め込むと、同じ名前の Go フィールドが 2 つできる
			name: "同じフラグメントのスプレッドは1つにする",
			args: args{
				query: `query Q { node { ...NodeName name ...NodeName } } fragment NodeName on Node { name }`,
			},
			want: want{
				selections: "...NodeName name",
			},
		},
		{
			// 選択が無い (空の) 選択セットでも panic しない
			name: "空の選択セットは空のまま返す",
			args: args{
				query: ``,
			},
			want: want{
				selections: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var selectionSet graphql.SelectionSet
			if tt.args.query != "" {
				// 検証の規則に nil を渡すと、既定の規則で検証する (queryparser と同じ呼び方)
				document, errs := gqlparser.LoadQueryWithRules(schema, tt.args.query, nil)
				if errs != nil {
					t.Fatalf("failed to load query: %v", errs)
				}
				node, ok := document.Operations[0].SelectionSet[0].(*graphql.Field)
				if !ok {
					t.Fatalf("first selection of the operation is not a field: %T", document.Operations[0].SelectionSet[0])
				}
				selectionSet = node.SelectionSet
			}
			source := formatSelections(selectionSet)

			got := mergeSelections(selectionSet)

			if diff := cmp.Diff(tt.want.selections, formatSelections(got)); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}

			// AST はサーバーへ送る Document の生成にも使うため、まとめる前の選択セットを書き換えない
			if diff := cmp.Diff(source, formatSelections(selectionSet)); diff != "" {
				t.Errorf("source selection set was modified, diff(-want +got): %s", diff)
			}
		})
	}
}
