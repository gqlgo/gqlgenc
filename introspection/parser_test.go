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

			ast := SchemaFromIntrospection("test", query)
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

	doc := SchemaFromIntrospection("test", query)
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
}
