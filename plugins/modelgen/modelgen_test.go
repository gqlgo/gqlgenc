package modelgen

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin/modelgen"

	"github.com/Yamashou/gqlgenc/v3/config"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestMutateHook(t *testing.T) {
	t.Parallel()

	schema := gqlparser.MustLoadSchema(&ast.Source{Input: `
type Query {
	used: UsedObject
}

input UsedInput {
	a: String
}

input UnusedInput {
	b: String
}

enum UsedEnum {
	A
}

enum UnusedEnum {
	B
}

type UsedObject {
	c: String
}
`})

	type args struct {
		usedTypes map[string]bool
		build     *modelgen.ModelBuild
	}

	type want struct {
		models     []string
		enums      []string
		interfaces []*modelgen.Interface
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// クライアントが参照するのは Input と Enum だけなので、使われている Input と Enum
			// のみを生成し、Object と Interface は生成しない
			name: "使われているInputとEnumだけ生成しObject・Interfaceは生成しない",
			args: args{
				usedTypes: map[string]bool{"UsedInput": true, "UsedEnum": true},
				build: &modelgen.ModelBuild{
					Models: []*modelgen.Object{
						{Name: "UsedInput"},
						{Name: "UnusedInput"},
						{Name: "UsedObject"},
					},
					Enums: []*modelgen.Enum{
						{Name: "UsedEnum"},
						{Name: "UnusedEnum"},
					},
					Interfaces: []*modelgen.Interface{
						{Name: "SomeInterface"},
					},
				},
			},
			want: want{
				models:     []string{"UsedInput"},
				enums:      []string{"UsedEnum"},
				interfaces: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{GQLGenConfig: &gqlgenconfig.Config{Schema: schema}}

			got := mutateHook(cfg, tt.args.usedTypes)(tt.args.build)

			var models []string
			for _, model := range got.Models {
				models = append(models, model.Name)
			}

			var enums []string
			for _, enum := range got.Enums {
				enums = append(enums, enum.Name)
			}

			if diff := cmp.Diff(tt.want.models, models); diff != "" {
				t.Errorf("models diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.enums, enums); diff != "" {
				t.Errorf("enums diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.interfaces, got.Interfaces); diff != "" {
				t.Errorf("interfaces diff(-want +got): %s", diff)
			}
		})
	}
}
