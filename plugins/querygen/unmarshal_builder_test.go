package querygen

import (
	"fmt"
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
				// 親のUnmarshal statement + 子のUnmarshal statement = 2
				statementsCount: 2,
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
		regularFieldsCount    int
		fragmentSpreadsCount  int
		inlineFragmentsCount  int
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

func TestUnmarshalBuilder_createFragmentUnmarshalStmt(t *testing.T) {
	t.Parallel()

	type args struct {
		field FieldInfo
	}

	type want struct {
		statementType string
		contains      string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "通常のfragment fieldからErrorCheckStatementを生成できることを確認する",
			args: args{
				field: FieldInfo{
					Name: "UserFragment",
				},
			},
			want: want{
				statementType: "*querygen.ErrorCheckStatement",
				contains:      "json.Unmarshal(data, &t.UserFragment)",
			},
		},
		{
			name: "ネストしたfragment fieldからもErrorCheckStatementを生成できることを確認する",
			args: args{
				field: FieldInfo{
					Name: "NestedFragment",
				},
			},
			want: want{
				statementType: "*querygen.ErrorCheckStatement",
				contains:      "json.Unmarshal(data, &t.NestedFragment)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.createFragmentUnmarshalStmt(tt.args.field)

			if diff := cmp.Diff(tt.want.statementType, fmt.Sprintf("%T", got)); diff != "" {
				t.Errorf("statement type diff(-want +got): %s", diff)
			}

			// String() メソッドで期待する文字列が含まれていることを確認
			gotString := got.String(0)
			if !strings.Contains(gotString, tt.want.contains) {
				t.Errorf("statement does not contain expected string: want %q in %q", tt.want.contains, gotString)
			}
		})
	}
}

// cmpStatement は Statement の比較用カスタム comparator
func cmpStatement(x, y Statement) bool {
	if x == nil && y == nil {
		return true
	}
	if x == nil || y == nil {
		return false
	}

	// String() メソッドで比較（同じindentで）
	return x.String(0) == y.String(0)
}

func TestUnmarshalBuilder_decodeSingleFragmentSpread(t *testing.T) {
	t.Parallel()

	type args struct {
		field FieldInfo
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
			name: "SubFieldsがない単純なfragment spreadの場合はUnmarshal statementのみ生成される",
			args: args{
				field: FieldInfo{
					Name:       "UserFragment",
					IsEmbedded: true,
					JSONTag:    "-",
					SubFields:  []FieldInfo{},
				},
			},
			want: want{
				statementsCount: 1, // Unmarshal statement のみ
			},
		},
		{
			name: "SubFieldsがあるfragment spreadの場合は再帰処理が実行される",
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
			},
			want: want{
				// Unmarshal statement + SubFieldsのUnmarshal statement = 2
				statementsCount: 2,
			},
		},
		{
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
			},
			want: want{
				// Unmarshal statement + inline fragment処理(VariableDecl + IfStatement + SwitchStatement) = 4
				statementsCount: 4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.decodeSingleFragmentSpread(tt.args.field)

			if diff := cmp.Diff(tt.want.statementsCount, len(got)); diff != "" {
				t.Errorf("statements count diff(-want +got): %s", diff)
			}

			// 最初の statement は必ず ErrorCheckStatement であることを確認
			if len(got) > 0 {
				if _, ok := got[0].(*ErrorCheckStatement); !ok {
					t.Errorf("first statement is not ErrorCheckStatement, got: %T", got[0])
				}
			}
		})
	}
}

func TestUnmarshalBuilder_decodeNestedFields(t *testing.T) {
	t.Parallel()

	type args struct {
		parentField FieldInfo
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
			name: "SubFieldsが空の場合は空のstatementsを返す",
			args: args{
				parentField: FieldInfo{
					Name:       "ParentFragment",
					IsEmbedded: true,
					JSONTag:    "-",
					SubFields:  []FieldInfo{},
				},
			},
			want: want{
				statementsCount: 0,
			},
		},
		{
			name: "SubFieldsにfragment spreadが含まれる場合は再帰的に処理される",
			args: args{
				parentField: FieldInfo{
					Name:       "ParentFragment",
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
			want: want{
				statementsCount: 1, // NestedFragmentのUnmarshal statement
			},
		},
		{
			name: "SubFieldsにinline fragmentが含まれる場合は処理される",
			args: args{
				parentField: FieldInfo{
					Name:       "ParentFragment",
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
			},
			want: want{
				// inline fragment処理(VariableDecl + IfStatement + SwitchStatement) = 3
				statementsCount: 3,
			},
		},
		{
			name: "SubFieldsにfragment spreadとinline fragmentが混在する場合も正しく処理される",
			args: args{
				parentField: FieldInfo{
					Name:       "ParentFragment",
					IsEmbedded: true,
					JSONTag:    "-",
					SubFields: []FieldInfo{
						{
							Name:       "NestedFragment",
							IsEmbedded: true,
							JSONTag:    "-",
							SubFields:  []FieldInfo{},
						},
						{
							Name:             "InlineFragment",
							IsInlineFragment: true,
							IsPointer:        true,
							PointerElemType:  "SomeType",
						},
					},
				},
			},
			want: want{
				// fragment spreadのUnmarshal + inline fragment処理(VariableDecl + IfStatement + SwitchStatement) = 4
				statementsCount: 4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.decodeNestedFields(tt.args.parentField)

			if diff := cmp.Diff(tt.want.statementsCount, len(got)); diff != "" {
				t.Errorf("statements count diff(-want +got): %s", diff)
			}
		})
	}
}

func TestUnmarshalBuilder_BuildUnmarshalMethod(t *testing.T) {
	type args struct {
		fields []FieldInfo
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
			name: "通常のフィールドのみの型の場合",
			args: args{
				fields: []FieldInfo{
					{
						Name:    "ID",
						JSONTag: "id",
					},
					{
						Name:    "Name",
						JSONTag: "name",
					},
				},
			},
			want: want{
				// VariableDecl + Unmarshal + Return = 最低3個
				statementsCount: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewUnmarshalBuilder()
			got := b.BuildUnmarshalMethod(tt.args.fields)

			if len(got) < tt.want.statementsCount {
				t.Errorf("statements count = %d, want at least %d", len(got), tt.want.statementsCount)
			}

			// 最初は VariableDecl であることを確認
			if _, ok := got[0].(*VariableDecl); !ok {
				t.Errorf("first statement is not VariableDecl, got: %T", got[0])
			}

			// 最後は ReturnStatement であることを確認
			if _, ok := got[len(got)-1].(*ReturnStatement); !ok {
				t.Errorf("last statement is not ReturnStatement, got: %T", got[len(got)-1])
			}
		})
	}
}
