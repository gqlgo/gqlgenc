package introspection

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// introspectionQueryErrors は、document を introspection のスキーマに対して検証し、検証エラーの文面を返す。
// 妥当なクエリなら "" を返す。
func introspectionQueryErrors(t *testing.T, document string) string {
	t.Helper()

	// introspection の型 (__Schema・__Type など) は、gqlparser が組み込みのスキーマとして足す
	schema := gqlparser.MustLoadSchema(&ast.Source{Input: "type Query { test: Boolean }"})

	_, errs := gqlparser.LoadQueryWithRules(schema, document, nil)

	return errs.Error()
}

func TestNewQuery(t *testing.T) {
	t.Parallel()

	// part は、1つの capability が Introspection に対して変える部分
	type part struct {
		from string
		to   string
	}

	fieldArguments := part{
		from: "\n        args {",
		to:   "\n        args(includeDeprecated: true) {",
	}
	inputFields := part{
		from: "\n      inputFields {",
		to:   "\n      inputFields(includeDeprecated: true) {",
	}
	directiveArguments := part{
		from: "\n          args {",
		to:   "\n          args(includeDeprecated: true) {",
	}
	inputValueDeprecation := part{
		from: "      defaultValue\n    }",
		to:   "      defaultValue\n      isDeprecated\n      deprecationReason\n    }",
	}
	repeatableDirective := part{
		from: "          description\n          locations",
		to:   "          description\n          isRepeatable\n          locations",
	}

	// queryWith は、Introspection に1回だけ現れる各 part の from を to に置き換えたクエリを返す
	queryWith := func(parts ...part) string {
		query := Introspection
		for _, p := range parts {
			if count := strings.Count(query, p.from); count != 1 {
				t.Fatalf("%q appears %d times in the query, want 1", p.from, count)
			}
			query = strings.Replace(query, p.from, p.to, 1)
		}

		return query
	}

	type args struct {
		capabilities Capabilities
	}

	type want struct {
		// query は組み立てたクエリ。Introspection から、capability に対応する部分だけが変わる
		query string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "capability が無ければ Introspection と同じクエリになる",
			args: args{capabilities: Capabilities{}},
			want: want{query: queryWith()},
		},
		{
			name: "FieldArgumentDeprecation はフィールドの引数に includeDeprecated を足す",
			args: args{capabilities: Capabilities{FieldArgumentDeprecation: true}},
			want: want{query: queryWith(fieldArguments)},
		},
		{
			name: "InputFieldDeprecation は input フィールドに includeDeprecated を足す",
			args: args{capabilities: Capabilities{InputFieldDeprecation: true}},
			want: want{query: queryWith(inputFields)},
		},
		{
			name: "DirectiveArgumentDeprecation はディレクティブの引数に includeDeprecated を足す",
			args: args{capabilities: Capabilities{DirectiveArgumentDeprecation: true}},
			want: want{query: queryWith(directiveArguments)},
		},
		{
			name: "InputValueDeprecation は引数と input フィールドの isDeprecated と deprecationReason を足す",
			args: args{capabilities: Capabilities{InputValueDeprecation: true}},
			want: want{query: queryWith(inputValueDeprecation)},
		},
		{
			name: "RepeatableDirective はディレクティブの isRepeatable を足す",
			args: args{capabilities: Capabilities{RepeatableDirective: true}},
			want: want{query: queryWith(repeatableDirective)},
		},
		{
			name: "すべての capability があれば、すべての部分を足す",
			args: args{capabilities: Capabilities{
				FieldArgumentDeprecation:     true,
				InputFieldDeprecation:        true,
				DirectiveArgumentDeprecation: true,
				InputValueDeprecation:        true,
				RepeatableDirective:          true,
			}},
			want: want{query: queryWith(fieldArguments, inputFields, directiveArguments, inputValueDeprecation, repeatableDirective)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewQuery(tt.args.capabilities)

			if diff := cmp.Diff(tt.want.query, got); diff != "" {
				t.Errorf("query diff(-want +got): %s", diff)
			}

			// どの capability の組み合わせでも、introspection のスキーマに対して妥当なクエリになる
			if diff := cmp.Diff("", introspectionQueryErrors(t, got)); diff != "" {
				t.Errorf("query validation errors diff(-want +got): %s", diff)
			}
		})
	}
}
