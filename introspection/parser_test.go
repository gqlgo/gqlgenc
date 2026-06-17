package introspection

import (
	json "encoding/json/v2"
	"os"
	"testing"

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
