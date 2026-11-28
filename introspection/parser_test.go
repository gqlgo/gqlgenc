package introspection

import (
	json "encoding/json/v2"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/vektah/gqlparser/v2/ast"
)

func TestSchemaFromIntrospection_Parse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "no mutation in schema",
			filename: "testdata/introspection_result_no_mutation.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("parseIntrospectionQuery() panicked: %v", r)
				}
			}()

			query := readQueryResult(t, tt.filename)

			ast, err := SchemaFromIntrospection("test", query)
			if err != nil {
				t.Fatalf("SchemaFromIntrospection() error = %v", err)
			}
			if ast == nil {
				t.Error("SchemaFromIntrospection() returned nil")
			}
		})
	}
}

func readQueryResult(t *testing.T, filename string) Query {
	t.Helper()

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Error reading file %s: %v", filename, err)
	}

	query := Query{}

	err = json.Unmarshal(data, &query)
	if err != nil {
		t.Fatalf("Error unmarshaling JSON: %v", err)
	}

	return query
}

// deprecation は、スキーマの要素に付いた @deprecated の内容。
type deprecation struct {
	// Reason は reason 引数の値。引数が無ければ nil
	Reason *string
	// Location は、ディレクティブが付いた要素の種類
	Location ast.DirectiveLocation
}

// findDeprecation は、directives の中の @deprecated の内容を返す。@deprecated が無ければ nil を返す。
func findDeprecation(directives ast.DirectiveList) *deprecation {
	deprecated := directives.ForName("deprecated")
	if deprecated == nil {
		return nil
	}

	found := &deprecation{Location: deprecated.Location}
	if reason := deprecated.Arguments.ForName("reason"); reason != nil {
		found.Reason = &reason.Value.Raw
	}

	return found
}

func TestSchemaFromIntrospection_AllTypeKinds(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SchemaFromIntrospection() panicked: %v", r)
		}
	}()

	query := readQueryResult(t, "testdata/introspection_result_all_kinds.json")

	doc, err := SchemaFromIntrospection("test", query)
	if err != nil {
		t.Fatalf("SchemaFromIntrospection() error = %v", err)
	}
	if doc == nil {
		t.Fatal("SchemaFromIntrospection() returned nil")
	}

	// 各 introspection の型種別が、対応する ast.Definition の Kind に変換されることを確認する
	kindByName := make(map[string]ast.DefinitionKind, len(doc.Definitions))
	for _, def := range doc.Definitions {
		kindByName[def.Name] = def.Kind
	}

	wantKinds := map[string]ast.DefinitionKind{
		"Query":        ast.Object,
		"SearchResult": ast.Union,
		"Status":       ast.Enum,
		"CreateInput":  ast.InputObject,
		"DateTime":     ast.Scalar,
	}
	for name, want := range wantKinds {
		if got := kindByName[name]; got != want {
			t.Errorf("%s: kind = %q, want %q", name, got, want)
		}
	}

	// federation/カスタムの directive 定義が取り込まれること
	if doc.Directives.ForName("myDirective") == nil {
		t.Error("myDirective directive definition was not parsed")
	}

	// 各引数のデフォルト値が、型に応じた ast.ValueKind に変換されることを確認する
	queryDef := doc.Definitions.ForName("Query")
	if queryDef == nil {
		t.Fatal("Query definition was not parsed")
	}
	variants := queryDef.Fields.ForName("variants")
	if variants == nil {
		t.Fatal("Query.variants field was not parsed")
	}
	wantArgKinds := map[string]ast.ValueKind{
		"s":   ast.StringValue,
		"b":   ast.BooleanValue,
		"fl":  ast.FloatValue,
		"st":  ast.EnumValue,
		"dt":  ast.StringValue,
		"inp": ast.ObjectValue,
	}
	for argName, want := range wantArgKinds {
		arg := variants.Arguments.ForName(argName)
		if arg == nil || arg.DefaultValue == nil {
			t.Errorf("%s: default value was not parsed", argName)
			continue
		}
		if got := arg.DefaultValue.Kind; got != want {
			t.Errorf("%s: default value kind = %v, want %v", argName, got, want)
		}
	}

	// 文字列の既定値は、サーバーが返すリテラル ("hello") の引用符を外した値になる
	if s := variants.Arguments.ForName("s"); s != nil && s.DefaultValue != nil {
		if diff := cmp.Diff("hello", s.DefaultValue.Raw); diff != "" {
			t.Errorf("s: default value raw diff(-want +got): %s", diff)
		}
	}

	// input フィールドの既定値が取り込まれる (落とすと、既定値を持つ必須フィールドを省いたクエリが検証で弾かれる)
	createInput := doc.Definitions.ForName("CreateInput")
	if createInput == nil {
		t.Fatal("CreateInput definition was not parsed")
	}
	wantInputDefaults := map[string]*ast.Value{
		"name":  nil,
		"count": {Raw: "3", Kind: ast.IntValue},
	}
	for fieldName, want := range wantInputDefaults {
		field := createInput.Fields.ForName(fieldName)
		if field == nil {
			t.Errorf("CreateInput.%s field was not parsed", fieldName)
			continue
		}
		if diff := cmp.Diff(want, field.DefaultValue, cmpopts.IgnoreFields(ast.Value{}, "Position", "Comment")); diff != "" {
			t.Errorf("CreateInput.%s: default value diff(-want +got): %s", fieldName, diff)
		}
	}

	// deprecated な要素 (enum の値・引数・input フィールド) にだけ @deprecated が付き、理由が reason 引数に、
	// 要素の種類が Location に入る。引数と input フィールドの @deprecated を落とすと、それらを使うクエリが
	// 検証で弾かれる
	status := doc.Definitions.ForName("Status")
	if status == nil {
		t.Fatal("Status definition was not parsed")
	}
	gotDeprecations := make(map[string]*deprecation)
	for _, enumValue := range status.EnumValues {
		gotDeprecations["Status."+enumValue.Name] = findDeprecation(enumValue.Directives)
	}
	for _, argument := range variants.Arguments {
		gotDeprecations["Query.variants("+argument.Name+":)"] = findDeprecation(argument.Directives)
	}
	for _, inputField := range createInput.Fields {
		gotDeprecations["CreateInput."+inputField.Name] = findDeprecation(inputField.Directives)
	}
	reason := func(text string) *string { return &text }
	wantDeprecations := map[string]*deprecation{
		"Status.ACTIVE":        nil,
		"Status.INACTIVE":      {Reason: reason("gone"), Location: ast.LocationEnumValue},
		"Query.variants(s:)":   nil,
		"Query.variants(b:)":   nil,
		"Query.variants(fl:)":  nil,
		"Query.variants(st:)":  nil,
		"Query.variants(dt:)":  nil,
		"Query.variants(inp:)": nil,
		"Query.variants(old:)": {Reason: reason("use s"), Location: ast.LocationArgumentDefinition},
		"CreateInput.name":     nil,
		"CreateInput.count":    nil,
		"CreateInput.legacy":   {Reason: reason("use count"), Location: ast.LocationInputFieldDefinition},
	}
	if diff := cmp.Diff(wantDeprecations, gotDeprecations); diff != "" {
		t.Errorf("deprecations diff(-want +got): %s", diff)
	}

	// repeatable なディレクティブだけが repeatable になる (落とすと、同じ位置に2回書いたクエリが検証で弾かれる)
	gotRepeatable := make(map[string]bool, len(doc.Directives))
	for _, directive := range doc.Directives {
		gotRepeatable[directive.Name] = directive.IsRepeatable
	}
	wantRepeatable := map[string]bool{
		"myDirective": true,
		"once":        false,
	}
	if diff := cmp.Diff(wantRepeatable, gotRepeatable); diff != "" {
		t.Errorf("repeatable directives diff(-want +got): %s", diff)
	}

	// queryType / mutationType / subscriptionType が schema 定義の operation type として取り込まれる
	gotOps := make(map[ast.Operation]string)
	for _, ot := range doc.Schema[0].OperationTypes {
		gotOps[ot.Operation] = ot.Type
	}
	wantOps := map[ast.Operation]string{
		ast.Query:        "Query",
		ast.Mutation:     "Mutation",
		ast.Subscription: "Subscription",
	}
	for op, want := range wantOps {
		if got := gotOps[op]; got != want {
			t.Errorf("operation %q type = %q, want %q", op, got, want)
		}
	}
}

func TestParser_buildDefaultValue(t *testing.T) {
	t.Parallel()

	position := &ast.Position{Src: &ast.Source{Name: "remote"}}

	typeName := func(name string) *string { return &name }
	p := parser{
		sharedPosition: position,
		typeMap: map[string]*FullType{
			"Int":        {Kind: TypeKindScalar, Name: typeName("Int")},
			"String":     {Kind: TypeKindScalar, Name: typeName("String")},
			"ID":         {Kind: TypeKindScalar, Name: typeName("ID")},
			"Status":     {Kind: TypeKindEnum, Name: typeName("Status")},
			"RangeInput": {Kind: TypeKindInputObject, Name: typeName("RangeInput")},
		},
	}

	literal := func(s string) *string { return &s }

	type args struct {
		defaultValue *string
		typ          *ast.Type
	}

	type want struct {
		value *ast.Value
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 既定値を持たない引数・input フィールドは nil のままにする
			name: "既定値が無ければnilを返す",
			args: args{
				defaultValue: nil,
				typ:          ast.NamedType("String", nil),
			},
			want: want{
				value: nil,
			},
		},
		{
			// 引用符を残すと、スキーマを書き出したときに "\"default \\\"name\\\"\"" と二重に引用される
			name: "文字列のリテラルは引用符とエスケープを外した値になる",
			args: args{
				defaultValue: literal(`"default \"name\""`),
				typ:          ast.NamedType("String", nil),
			},
			want: want{
				value: &ast.Value{Raw: `default "name"`, Kind: ast.StringValue, Position: position},
			},
		},
		{
			// リストを 1 つの enum の値として持つと、要素が失われる
			name: "リストのリテラルは要素を保つ",
			args: args{
				defaultValue: literal(`[ACTIVE, ARCHIVED]`),
				typ:          ast.ListType(ast.NonNullNamedType("Status", nil), nil),
			},
			want: want{
				value: &ast.Value{
					Kind:     ast.ListValue,
					Position: position,
					Children: ast.ChildValueList{
						{Value: &ast.Value{Raw: "ACTIVE", Kind: ast.EnumValue, Position: position}, Position: position},
						{Value: &ast.Value{Raw: "ARCHIVED", Kind: ast.EnumValue, Position: position}, Position: position},
					},
				},
			},
		},
		{
			// オブジェクトをフィールドの無い値として持つと、{} として書き出される
			name: "オブジェクトのリテラルはフィールドを保つ",
			args: args{
				defaultValue: literal(`{min: 1, max: 10}`),
				typ:          ast.NamedType("RangeInput", nil),
			},
			want: want{
				value: &ast.Value{
					Kind:     ast.ObjectValue,
					Position: position,
					Children: ast.ChildValueList{
						{Name: "min", Value: &ast.Value{Raw: "1", Kind: ast.IntValue, Position: position}, Position: position},
						{Name: "max", Value: &ast.Value{Raw: "10", Kind: ast.IntValue, Position: position}, Position: position},
					},
				},
			},
		},
		{
			// 既定値が null のとき、型の種類 (文字列) の値 "null" にしない
			name: "nullのリテラルはnullの値になる",
			args: args{
				defaultValue: literal(`null`),
				typ:          ast.NamedType("ID", nil),
			},
			want: want{
				value: &ast.Value{Raw: "null", Kind: ast.NullValue, Position: position},
			},
		},
		{
			// サーバーが GraphQL のリテラルでない文字列を返しても、エラーにせず書かれたまま残す
			name: "リテラルとして解釈できない値は、型の種類の値として書かれたまま残す",
			args: args{
				defaultValue: literal(`{min: `),
				typ:          ast.NamedType("RangeInput", nil),
			},
			want: want{
				value: &ast.Value{Raw: `{min: `, Kind: ast.ObjectValue, Position: position},
			},
		},
		{
			// 引数の括弧を閉じて別のフィールドを続ける文字列を、最初の値だけ採用して通さない
			name: "1つの値でない文字列は、書かれたまま残す",
			args: args{
				defaultValue: literal(`1) other(argument: 2`),
				typ:          ast.NamedType("Int", nil),
			},
			want: want{
				value: &ast.Value{Raw: `1) other(argument: 2`, Kind: ast.IntValue, Position: position},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := p.buildDefaultValue(tt.args.defaultValue, tt.args.typ)

			// Comment は gqlparser がリテラルの前のコメントを入れる欄で、既定値の内容ではないため比較から外す
			opts := []cmp.Option{
				cmpopts.IgnoreFields(ast.Value{}, "Comment"),
				cmpopts.IgnoreFields(ast.ChildValue{}, "Comment"),
			}
			if diff := cmp.Diff(tt.want.value, got, opts...); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

// TestSchemaFromIntrospection_TypeTooDeep は、型の入れ子が introspection クエリの ofType 深さ
// を超えて切り詰められた (OfType=nil) 応答で、panic ではなくエラーが返ることを確認する。
// これは仕様違反の応答ではなく valid なスキーマで起き得る唯一のケースで、recover で救う対象。
func TestSchemaFromIntrospection_TypeTooDeep(t *testing.T) {
	t.Parallel()

	queryName := "Query"
	var query Query
	query.Schema.QueryType.Name = &queryName
	query.Schema.Types = FullTypes{
		{
			Kind: TypeKindObject,
			Name: &queryName,
			Fields: []*FieldValue{
				{
					Name: "deep",
					// ofType 深さ超過で切り詰められた型を模す (LIST なのに OfType が無い)。
					Type: TypeRef{Kind: TypeKindList, OfType: nil},
				},
			},
		},
	}

	_, err := SchemaFromIntrospection("test", query)
	if err == nil {
		t.Fatal("SchemaFromIntrospection() should return an error for a type deeper than the introspection query, got nil")
	}
}
