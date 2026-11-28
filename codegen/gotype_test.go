package codegen

import (
	"errors"
	gotypes "go/types"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/internal/cmputil"
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
			// $x が偽のとき、payload は返るが success は返らない。success を条件付きにしないと、
			// 非 null の bool で生成され、返らなかった success を false と読み違える
			name: "条件付きの選択と条件の無い選択をまとめると、条件付きの選択のサブフィールドを条件付きにする",
			args: args{
				query: `query Q($x: Boolean!) { node { payload @include(if: $x) { success } payload { message } } }`,
			},
			want: want{
				selections: "payload{success@include message}",
			},
		},
		{
			// payload はどちらかの条件が真なら返るが、そのとき反対の条件の選択のサブフィールドは返らない
			name: "条件の違う選択をまとめると、それぞれのサブフィールドにその選択の条件を付ける",
			args: args{
				query: `query Q($x: Boolean!) { node { payload @include(if: $x) { success } payload @skip(if: $x) { message } } }`,
			},
			want: want{
				selections: "payload@include{success@include message@skip}",
			},
		},
		{
			// 条件が同じなら、payload が返るときはどちらの選択のサブフィールドも返る
			name: "同じ条件の選択をまとめると、サブフィールドに条件を付けない",
			args: args{
				query: `query Q($x: Boolean!) { node { payload @include(if: $x) { success } payload @include(if: $x) { message } } }`,
			},
			want: want{
				selections: "payload@include{success message}",
			},
		},
		{
			// インラインフラグメントの中のフィールドも、条件が偽の選択のものは返らない
			name: "条件付きの選択のインラインフラグメントは、その中のフィールドに条件を付ける",
			args: args{
				query: `query Q($x: Boolean!) { node { payload @include(if: $x) { ... on Payload { success } } payload { message } } }`,
			},
			want: want{
				selections: "payload{...on Payload{success@include} message}",
			},
		},
		{
			// 同じ型への条件を別々の型にすると、同じ名前の Go 型を 2 回作って先の選択を失う
			name: "同じ型へのインラインフラグメントは1つにまとめる",
			args: args{
				query: `query Q { node { ... on Item { title } ... on Comment { body } ... on Item { name } } }`,
			},
			want: want{
				selections: "...on Item{title name} ...on Comment{body}",
			},
		},
		{
			// 型条件が空のままだと、空の名前のフィールドと型を作ってコンパイルできないコードになる
			name: "型条件の無いインラインフラグメントは、外側の型への条件にしてまとめる",
			args: args{
				query: `query Q($x: Boolean!) { node { ... { name } ... @include(if: $x) { payload { success } } ... on Node { id } } }`,
			},
			want: want{
				selections: "...on Node{name payload{success} id}",
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

func TestGoTypeGenerator_possibleTypeNames(t *testing.T) {
	t.Parallel()

	schema := gqlparser.MustLoadSchema(&graphql.Source{Input: `
		type Query {
			node: Node
		}
		interface Node {
			id: ID!
		}
		interface File implements Node {
			id: ID!
			filename: String!
		}
		type Image implements Node & File {
			id: ID!
			filename: String!
		}
		type Document implements Node & File {
			id: ID!
			filename: String!
		}
		type Item implements Node {
			id: ID!
		}
		type HttpError {
			message: String!
		}
		union SearchResult = Item | Image | HttpError
	`})

	type args struct {
		typeCondition string
		enclosing     *graphql.Definition
	}

	type want struct {
		typeNames []string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// Object 型への型条件は、その型の名前がそのまま __typename になる
			name: "Object型はそれ自身を返す",
			args: args{
				typeCondition: "Image",
				enclosing:     schema.Types["Node"],
			},
			want: want{
				typeNames: []string{"Image"},
			},
		},
		{
			// File は Node を実装する interface だが、__typename にはならないので含めない
			name: "interfaceは実装するObject型を名前順で返す",
			args: args{
				typeCondition: "Node",
				enclosing:     schema.Types["Node"],
			},
			want: want{
				typeNames: []string{"Document", "Image", "Item"},
			},
		},
		{
			// union のメンバーは、スキーマに書いた順ではなく名前順で返す
			name: "unionはメンバーを名前順で返す",
			args: args{
				typeCondition: "SearchResult",
				enclosing:     schema.Types["SearchResult"],
			},
			want: want{
				typeNames: []string{"HttpError", "Image", "Item"},
			},
		},
		{
			// File を返すフィールドの中の ... on Node は、File を実装していない Item にはならない
			name: "外側がinterfaceなら、その実装でない型を除く",
			args: args{
				typeCondition: "Node",
				enclosing:     schema.Types["File"],
			},
			want: want{
				typeNames: []string{"Document", "Image"},
			},
		},
		{
			// SearchResult を返すフィールドの中の ... on Node は、union のメンバーでない Document にはならない
			name: "外側がunionなら、メンバーでない型を除く",
			args: args{
				typeCondition: "Node",
				enclosing:     schema.Types["SearchResult"],
			},
			want: want{
				typeNames: []string{"Image", "Item"},
			},
		},
		{
			// 検証を通ったクエリでは起きないが、外側の型と重ならない型条件は具体型を持たない
			name: "外側の型になり得ない型条件は空を返す",
			args: args{
				typeCondition: "Item",
				enclosing:     schema.Types["File"],
			},
			want: want{
				typeNames: []string{},
			},
		},
		{
			// 外側の型が分からないときは、型条件だけで決める
			name: "外側の型がnilなら絞り込まない",
			args: args{
				typeCondition: "Node",
				enclosing:     nil,
			},
			want: want{
				typeNames: []string{"Document", "Image", "Item"},
			},
		},
		{
			name: "スキーマに無い型はnilを返す",
			args: args{
				typeCondition: "Unknown",
				enclosing:     nil,
			},
			want: want{
				typeNames: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := NewGoTypeGenerator(&config.Config{
				GQLGenConfig:  &gqlgenconfig.Config{Schema: schema},
				GQLGencConfig: &config.GQLGencConfig{},
			})
			got := g.possibleTypeNames(tt.args.typeCondition, tt.args.enclosing)

			if diff := cmp.Diff(tt.want.typeNames, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestFields_checkGoNameCollision(t *testing.T) {
	t.Parallel()

	type fields struct {
		fields Fields
	}

	type args struct {
		parentTypeName string
	}

	type want struct {
		err error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "Go名が重ならなければnilを返す",
			fields: fields{
				fields: Fields{
					{Name: "id", TypeKind: Scalar},
					{Name: "Item", TypeKind: InlineFragment},
					{Name: "ItemFields", TypeKind: FragmentSpread},
				},
			},
			args: args{
				parentTypeName: "Q_Node",
			},
			want: want{
				err: nil,
			},
		},
		{
			// 別名を付けずに選ぶと、どちらも Go のフィールド FooBar になる (gqlgo/gqlgenc#108)
			name: "名前の違うフィールドが同じGo名になるとエラーを返す",
			fields: fields{
				fields: Fields{
					{Name: "foo_bar", TypeKind: Scalar},
					{Name: "fooBar", TypeKind: Object},
				},
			},
			args: args{
				parentTypeName: "Q_Object",
			},
			want: want{
				err: errors.New(`field fooBar and field foo_bar in Q_Object map to the same Go field name "FooBar": add an alias to a field or rename a fragment in the query`),
			},
		},
		{
			// Item: id と ... on Item はどちらも Go のフィールド Item になり、片方が生成型から落ちる
			name: "フィールドの別名とインラインフラグメントの型条件が同じ名前ならエラーを返す",
			fields: fields{
				fields: Fields{
					{Name: "Item", TypeKind: Scalar},
					{Name: "Item", TypeKind: InlineFragment},
				},
			},
			args: args{
				parentTypeName: "Q_Node",
			},
			want: want{
				err: errors.New(`field Item and inline fragment on Item in Q_Node map to the same Go field name "Item": add an alias to a field or rename a fragment in the query`),
			},
		},
		{
			// ...Item と ... on Item はどちらも Go のフィールド Item になり、片方が生成型から落ちる
			name: "フラグメントの名前とインラインフラグメントの型条件が同じ名前ならエラーを返す",
			fields: fields{
				fields: Fields{
					{Name: "Item", TypeKind: InlineFragment},
					{Name: "Item", TypeKind: FragmentSpread},
				},
			},
			args: args{
				parentTypeName: "Q_Node",
			},
			want: want{
				err: errors.New(`fragment spread Item and inline fragment on Item in Q_Node map to the same Go field name "Item": add an alias to a field or rename a fragment in the query`),
			},
		},
		{
			// 同じ Go 型に束縛した 2 つのフラグメントは、同じ型を同じデータからデコードするので 1 つにまとめてよい
			name: "同じ種類の同じ名前の選択は衝突にしない",
			fields: fields{
				fields: Fields{
					{Name: "User", TypeKind: FragmentSpread},
					{Name: "User", TypeKind: FragmentSpread},
				},
			},
			args: args{
				parentTypeName: "Q_Node",
			},
			want: want{
				err: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fields.fields.checkGoNameCollision(tt.args.parentTypeName)

			// 裸の error は cmp.Comparer がトップレベルで効かないため、cmputil のヘルパーを真偽値で使う
			if !cmputil.EqualErrorMessage(tt.want.err, err) {
				t.Errorf("error = %v, want %v", err, tt.want.err)
			}
		})
	}
}
