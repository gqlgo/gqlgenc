package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestBindDefaultScalars(t *testing.T) {
	t.Parallel()

	const boundModel = "github.com/example/myapp/domain.Bound"

	type fields struct {
		schema string
		models gqlgenconfig.TypeMap
	}

	type want struct {
		typeName string
		model    []string
	}

	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			// 対応する Go 型が無い custom scalar は graphql.String を既定にする
			name: "対応型が無いcustom scalarはgraphql.Stringを既定にする",
			fields: fields{
				schema: "type Query { f: Unbound }\nscalar Unbound",
				models: gqlgenconfig.TypeMap{},
			},
			want: want{
				typeName: "Unbound",
				model:    []string{"github.com/99designs/gqlgen/graphql.String"},
			},
		},
		{
			// autobind / models: で既に束縛済みの scalar はその型を維持し graphql.String で上書きしない
			name: "既に束縛済みのcustom scalarは上書きせずその型を維持する",
			fields: fields{
				schema: "type Query { f: Bound }\nscalar Bound",
				models: gqlgenconfig.TypeMap{"Bound": {Model: gqlgenconfig.StringList{boundModel}}},
			},
			want: want{
				typeName: "Bound",
				model:    []string{boundModel},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := gqlparser.MustLoadSchema(&ast.Source{Input: tt.fields.schema})
			bindDefaultScalars(schema, tt.fields.models)

			// 本番では built-in scalar (String など) は injectBuiltins で先に束縛されるが、
			// このテストは bindDefaultScalars を単体で呼ぶため built-in も graphql.String 化される。
			// よって対象の custom scalar のみを検証する。
			if diff := cmp.Diff(tt.want.model, []string(tt.fields.models[tt.want.typeName].Model)); diff != "" {
				t.Errorf("%q model diff(-want +got): %s", tt.want.typeName, diff)
			}
		})
	}
}
