package clientgenv2

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReturnTypeName tests the returnTypeName function with various types.
func TestReturnTypeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    types.Type
		nested   bool
		expected string
		wantErr  string
	}{
		{
			name:     "Basic",
			input:    types.Typ[types.String],
			nested:   false,
			expected: "string",
		},
		{
			name:     "Pointer",
			input:    types.NewPointer(types.Typ[types.Int]),
			nested:   false,
			expected: "*int",
		},
		{
			name:     "Slice",
			input:    types.NewSlice(types.Typ[types.Float64]),
			nested:   false,
			expected: "[]float64",
		},
		{
			name:     "Named",
			input:    types.NewNamed(types.NewTypeName(0, nil, "MyType", nil), nil, nil),
			nested:   false,
			expected: "*MyType",
		},
		{
			name:     "Named from the universe scope",
			input:    types.Universe.Lookup("error").Type(),
			nested:   true,
			expected: "error",
		},
		{
			name:     "Interface",
			input:    types.NewInterfaceType(nil, nil).Complete(),
			nested:   false,
			expected: "any",
		},
		{
			name:     "Map",
			input:    types.NewMap(types.Typ[types.Int], types.Typ[types.Bool]),
			nested:   false,
			expected: "map[int]bool",
		},
		{
			name:    "Unsupported",
			input:   types.NewChan(types.SendRecv, types.Typ[types.Int]),
			nested:  false,
			wantErr: "unsupported type",
		},
	}

	g := &GenGettersGenerator{
		ClientPackageName: "hoge",
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			output, err := g.returnTypeName(test.input, test.nested)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expected, output)
		})
	}
}

// TestStructDef tests that struct tags are rendered as raw string literals.
func TestStructDef(t *testing.T) {
	t.Parallel()

	newField := func(name string, typ types.Type, embedded bool) *types.Var {
		return types.NewField(0, nil, name, typ, embedded)
	}
	named := types.NewNamed(types.NewTypeName(0, nil, "Fragment", nil), types.NewStruct(nil, nil), nil)
	lookupType := func(t types.Type) string { return types.TypeString(t, nil) }

	tests := []struct {
		name     string
		input    types.Type
		expected string
	}{
		{
			name: "Tags as raw string literals",
			input: types.NewStruct(
				[]*types.Var{
					newField("ID", types.Typ[types.String], false),
					newField("Items", types.NewSlice(types.NewPointer(named)), false),
				},
				[]string{`json:"id" graphql:"id"`, `json:"items" graphql:"items"`},
			),
			expected: "struct {\nID string `json:\"id\" graphql:\"id\"`\nItems []*Fragment `json:\"items\" graphql:\"items\"`\n}",
		},
		{
			name: "Tag containing a backtick keeps the quoted form",
			input: types.NewStruct(
				[]*types.Var{newField("Name", types.Typ[types.String], false)},
				[]string{"json:\"name\" doc:\"`name`\""},
			),
			expected: "struct {\nName string \"json:\\\"name\\\" doc:\\\"`name`\\\"\"\n}",
		},
		{
			name: "Embedded field and field without tag",
			input: types.NewStruct(
				[]*types.Var{
					newField("Fragment", named, true),
					newField("Count", types.Typ[types.Int], false),
				},
				nil,
			),
			expected: "struct {\nFragment\nCount int\n}",
		},
		{
			name:     "Non-struct types fall back to lookupType",
			input:    named,
			expected: "Fragment",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.expected, structDef(test.input, lookupType))
		})
	}
}
