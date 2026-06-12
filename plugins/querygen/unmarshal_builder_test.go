package querygen

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnmarshalBuilder_decodeFragmentSpreads(t *testing.T) {
	type args struct {
		fragmentSpreads []FieldInfo
	}

	type want struct {
		statementsCount int
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "空のfragment spreadsリストの場合は空のstatementsを返す",
			args: args{
				fragmentSpreads: []FieldInfo{},
			},
			want: want{
				statementsCount: 0,
			},
		},
		{
			name: "単一のfragment spreadを処理できることを確認する",
			args: args{
				fragmentSpreads: []FieldInfo{
					{
						Name:       "UserFragment",
						IsEmbedded: true,
						JSONTag:    "-",
						SubFields:  []FieldInfo{},
					},
				},
			},
			want: want{
				statementsCount: 1,
			},
		},
		{
			name: "ネストしたfragment spreadを再帰的に処理できることを確認する",
			args: args{
				fragmentSpreads: []FieldInfo{
					{
						Name:       "UserFragment",
						IsEmbedded: true,
						JSONTag:    "-",
						SubFields: []FieldInfo{
							{
								Name:       "NestedFragment",
								IsEmbedded: true,
								JSONTag:    "-",
								SubFields:  []FieldInfo{},
							},
						},
					},
				},
			},
			want: want{
				// 親は通常フィールドを持たないため、子のフォールバック statement のみ = 1
				statementsCount: 1,
			},
		},
		{
			name: "複数のfragment spreadsを処理できることを確認する",
			args: args{
				fragmentSpreads: []FieldInfo{
					{
						Name:       "UserFragment",
						IsEmbedded: true,
						JSONTag:    "-",
						SubFields:  []FieldInfo{},
					},
					{
						Name:       "PostFragment",
						IsEmbedded: true,
						JSONTag:    "-",
						SubFields:  []FieldInfo{},
					},
				},
			},
			want: want{
				statementsCount: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewUnmarshalBuilder()
			got := b.decodeFragmentSpreads(tt.args.fragmentSpreads)

			if diff := cmp.Diff(tt.want.statementsCount, len(got)); diff != "" {
				t.Errorf("statements count diff(-want +got): %s", diff)
			}

			// 各 statement が ErrorCheckStatement であることを確認
			for i, stmt := range got {
				if _, ok := stmt.(*ErrorCheckStatement); !ok {
					t.Errorf("statement[%d] is not ErrorCheckStatement, got: %T", i, stmt)
				}
			}
		})
	}
}

func TestUnmarshalBuilder_separateFieldTypesAt(t *testing.T) {
	t.Parallel()

	type args struct {
		fields     []FieldInfo
		parentPath string
	}

	type want struct {
		regularFieldsCount   int
		fragmentSpreadsCount int
		inlineFragmentsCount int
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "通常のフィールドのみの場合",
			args: args{
				fields: []FieldInfo{
					{
						Name:       "ID",
						IsEmbedded: false,
						JSONTag:    "id",
					},
					{
						Name:       "Name",
						IsEmbedded: false,
						JSONTag:    "name",
					},
				},
				parentPath: "t",
			},
			want: want{
				regularFieldsCount:   2,
				fragmentSpreadsCount: 0,
				inlineFragmentsCount: 0,
			},
		},
		{
			name: "fragment spreadフィールドを識別できることを確認する",
			args: args{
				fields: []FieldInfo{
					{
						Name:       "UserFragment",
						IsEmbedded: true,
						JSONTag:    "-",
					},
				},
				parentPath: "t",
			},
			want: want{
				regularFieldsCount:   0,
				fragmentSpreadsCount: 1,
				inlineFragmentsCount: 0,
			},
		},
		{
			name: "inline fragmentフィールドを識別できることを確認する",
			args: args{
				fields: []FieldInfo{
					{
						Name:             "Fragment",
						IsInlineFragment: true,
						IsPointer:        true,
						PointerElemType:  "UserFragment",
					},
				},
				parentPath: "t",
			},
			want: want{
				regularFieldsCount:   0,
				fragmentSpreadsCount: 0,
				inlineFragmentsCount: 1,
			},
		},
		{
			name: "混在したフィールドを正しく分類できることを確認する",
			args: args{
				fields: []FieldInfo{
					{
						Name:       "ID",
						IsEmbedded: false,
						JSONTag:    "id",
					},
					{
						Name:       "UserFragment",
						IsEmbedded: true,
						JSONTag:    "-",
					},
					{
						Name:             "InlineFragment",
						IsInlineFragment: true,
						IsPointer:        true,
						PointerElemType:  "SomeType",
					},
				},
				parentPath: "t",
			},
			want: want{
				regularFieldsCount:   1,
				fragmentSpreadsCount: 1,
				inlineFragmentsCount: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			regularFields, fragmentSpreads, inlineFragments := b.separateFieldTypesAt(tt.args.fields, tt.args.parentPath)

			if diff := cmp.Diff(tt.want.regularFieldsCount, len(regularFields)); diff != "" {
				t.Errorf("regularFields count diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.fragmentSpreadsCount, len(fragmentSpreads)); diff != "" {
				t.Errorf("fragmentSpreads count diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.inlineFragmentsCount, len(inlineFragments)); diff != "" {
				t.Errorf("inlineFragments count diff(-want +got): %s", diff)
			}
		})
	}
}

func TestUnmarshalBuilder_decodeSingleFragmentSpread(t *testing.T) {
	t.Parallel()

	type args struct {
		field      FieldInfo
		parentPath string
	}

	type want struct {
		statementsCount int
		contains        []string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// SubFields が解析できない場合は data 全体からのデコードにフォールバックする
			name: "SubFieldsがない場合はdata全体のデコードにフォールバックする",
			args: args{
				field: FieldInfo{
					Name:       "UserFragment",
					IsEmbedded: true,
					JSONTag:    "-",
					SubFields:  []FieldInfo{},
				},
				parentPath: "t",
			},
			want: want{
				statementsCount: 1,
				contains: []string{
					"json.Unmarshal(data, &t.UserFragment)",
				},
			},
		},
		{
			// フラグメントの通常フィールドは raw マップからメンバー単位でデコードする
			name: "通常のSubFieldsはrawマップからメンバー単位でデコードされる",
			args: args{
				field: FieldInfo{
					Name:       "UserFragment",
					IsEmbedded: true,
					JSONTag:    "-",
					SubFields: []FieldInfo{
						{
							Name:       "ID",
							JSONTag:    "id",
							IsExported: true,
						},
						{
							Name:       "Name",
							JSONTag:    "name",
							IsExported: true,
						},
					},
				},
				parentPath: "t",
			},
			want: want{
				statementsCount: 2,
				contains: []string{
					`value, ok := raw["id"]; ok`,
					"json.Unmarshal(value, &t.UserFragment.ID)",
					`value, ok := raw["name"]; ok`,
					"json.Unmarshal(value, &t.UserFragment.Name)",
				},
			},
		},
		{
			// ネストした fragment spread は親パスを連結して再帰処理される
			name: "ネストしたfragment spreadは再帰的に処理される",
			args: args{
				field: FieldInfo{
					Name:       "UserFragment",
					IsEmbedded: true,
					JSONTag:    "-",
					SubFields: []FieldInfo{
						{
							Name:       "NestedFragment",
							IsEmbedded: true,
							JSONTag:    "-",
							SubFields:  []FieldInfo{},
						},
					},
				},
				parentPath: "t",
			},
			want: want{
				statementsCount: 1,
				contains: []string{
					"json.Unmarshal(data, &t.UserFragment.NestedFragment)",
				},
			},
		},
		{
			// inline fragment は __typename による switch でデコードされる
			name: "SubFieldsにinline fragmentが含まれる場合も処理される",
			args: args{
				field: FieldInfo{
					Name:       "UserFragment",
					IsEmbedded: true,
					JSONTag:    "-",
					SubFields: []FieldInfo{
						{
							Name:             "InlineFragment",
							IsInlineFragment: true,
							IsPointer:        true,
							PointerElemType:  "SomeType",
						},
					},
				},
				parentPath: "t",
			},
			want: want{
				// inline fragment処理(VariableDecl + IfStatement + SwitchStatement) = 3
				statementsCount: 3,
				contains: []string{
					`typename, ok := raw["__typename"]; ok`,
					"t.UserFragment.InlineFragment = &SomeType{}",
					"json.Unmarshal(data, t.UserFragment.InlineFragment)",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.decodeSingleFragmentSpread(tt.args.field, tt.args.parentPath)

			if diff := cmp.Diff(tt.want.statementsCount, len(got)); diff != "" {
				t.Errorf("statements count diff(-want +got): %s", diff)
			}

			var buf strings.Builder
			for _, stmt := range got {
				buf.WriteString(stmt.String(1))
				buf.WriteString("\n")
			}
			gotString := buf.String()

			for _, contains := range tt.want.contains {
				if !strings.Contains(gotString, contains) {
					t.Errorf("generated code does not contain %q:\n%s", contains, gotString)
				}
			}
		})
	}
}

func TestUnmarshalBuilder_BuildUnmarshalMethod(t *testing.T) {
	t.Parallel()

	type args struct {
		fields []FieldInfo
	}

	type want struct {
		contains    []string
		notContains []string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// fragment を含まない型はトークンを1パスで読むストリーミングモードになる
			name: "通常のフィールドのみの型はストリーミングモードで生成される",
			args: args{
				fields: []FieldInfo{
					{
						Name:       "ID",
						JSONTag:    "id",
						IsExported: true,
					},
					{
						Name:       "Name",
						JSONTag:    "name",
						IsExported: true,
					},
				},
			},
			want: want{
				contains: []string{
					"dec.PeekKind() == 'n'",
					"tok, err := dec.ReadToken()",
					"for dec.PeekKind() != '}' {",
					"switch name.String() {",
					`case "id":`,
					"json.UnmarshalDecode(dec, &t.ID)",
					`case "name":`,
					"json.UnmarshalDecode(dec, &t.Name)",
					"default:",
					"dec.SkipValue()",
				},
				notContains: []string{
					"map[string]jsontext.Value",
					"dec.ReadValue()",
				},
			},
		},
		{
			// fragment spread を含む型は値全体を一度バッファするモードになる
			name: "fragment spreadを含む型はバッファモードで生成される",
			args: args{
				fields: []FieldInfo{
					{
						Name:       "ID",
						JSONTag:    "id",
						IsExported: true,
					},
					{
						Name:       "UserFragment",
						IsEmbedded: true,
						JSONTag:    "-",
					},
				},
			},
			want: want{
				contains: []string{
					"data, err := dec.ReadValue()",
					"var raw map[string]jsontext.Value",
					"json.Unmarshal(data, &raw)",
					`value, ok := raw["id"]; ok`,
					"json.Unmarshal(data, &t.UserFragment)",
				},
				notContains: []string{
					"switch name.String() {",
				},
			},
		},
		{
			// inline fragment を含む型もバッファモードになる
			name: "inline fragmentを含む型はバッファモードで生成される",
			args: args{
				fields: []FieldInfo{
					{
						Name:             "User",
						IsInlineFragment: true,
						IsPointer:        true,
						PointerElemType:  "UserFragment",
					},
				},
			},
			want: want{
				contains: []string{
					"data, err := dec.ReadValue()",
					"var raw map[string]jsontext.Value",
					`typename, ok := raw["__typename"]; ok`,
					"json.Unmarshal(data, t.User)",
				},
				notContains: []string{
					"switch name.String() {",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.BuildUnmarshalMethod(tt.args.fields)

			var buf strings.Builder
			for _, stmt := range got {
				buf.WriteString(stmt.String(1))
				buf.WriteString("\n")
			}
			gotString := buf.String()

			for _, contains := range tt.want.contains {
				if !strings.Contains(gotString, contains) {
					t.Errorf("generated code does not contain %q:\n%s", contains, gotString)
				}
			}

			for _, notContains := range tt.want.notContains {
				if strings.Contains(gotString, notContains) {
					t.Errorf("generated code unexpectedly contains %q:\n%s", notContains, gotString)
				}
			}

			// 最後は ReturnStatement であることを確認
			if _, ok := got[len(got)-1].(*ReturnStatement); !ok {
				t.Errorf("last statement is not ReturnStatement, got: %T", got[len(got)-1])
			}
		})
	}
}
