package querygen

import (
	"go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestFieldAnalyzer_AnalyzeFields(t *testing.T) {
	t.Parallel()

	analyzer := NewFieldAnalyzer()

	type args struct {
		structType              *types.Struct
		shouldGenerateUnmarshal func(*types.Named) bool
	}

	type want struct {
		fields []FieldInfo
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "通常のエクスポートフィールドを解析できることを確認する",
			args: args{
				structType: types.NewStruct(
					[]*types.Var{
						types.NewField(0, nil, "Name", types.Typ[types.String], false),
						types.NewField(0, nil, "Age", types.Typ[types.Int], false),
					},
					[]string{
						`json:"name"`,
						`json:"age"`,
					},
				),
				shouldGenerateUnmarshal: func(*types.Named) bool { return false },
			},
			want: want{
				fields: []FieldInfo{
					{
						Name:       "Name",
						JSONTag:    "name",
						IsExported: true,
						IsEmbedded: false,
					},
					{
						Name:       "Age",
						JSONTag:    "age",
						IsExported: true,
						IsEmbedded: false,
					},
				},
			},
		},
		{
			name: "インラインフラグメントフィールドを検出できることを確認する",
			args: args{
				structType: types.NewStruct(
					[]*types.Var{
						types.NewField(0, nil, "Fragment", types.NewPointer(types.Typ[types.String]), false),
					},
					[]string{
						`json:"-"`,
					},
				),
				shouldGenerateUnmarshal: func(*types.Named) bool { return false },
			},
			want: want{
				fields: []FieldInfo{
					{
						Name:             "Fragment",
						JSONTag:          "-",
						IsExported:       true,
						IsEmbedded:       false,
						IsInlineFragment: true,
						IsPointer:        true,
					},
				},
			},
		},
		{
			name: "空の構造体を解析しても空のフィールドリストを返す",
			args: args{
				structType:              types.NewStruct([]*types.Var{}, []string{}),
				shouldGenerateUnmarshal: func(*types.Named) bool { return false },
			},
			want: want{
				fields: []FieldInfo{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := analyzer.AnalyzeFields(tt.args.structType, tt.args.shouldGenerateUnmarshal)

			// Type フィールドは比較から除外（go/types の内部表現の比較が複雑なため）
			opts := cmpopts.IgnoreFields(FieldInfo{}, "Type", "TypeName", "PointerElemType", "SubFields")
			if diff := cmp.Diff(tt.want.fields, got, opts); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestFieldAnalyzer_IsInlineFragment(t *testing.T) {
	t.Parallel()

	analyzer := NewFieldAnalyzer()

	type args struct {
		field *types.Var
		tag   string
	}

	type want struct {
		isInlineFragment bool
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "エクスポートされたポインタ型フィールドでJSONタグがない場合はインラインフラグメント",
			args: args{
				field: types.NewField(0, nil, "TestField", types.NewPointer(types.Typ[types.String]), false),
				tag:   "",
			},
			want: want{
				isInlineFragment: true,
			},
		},
		{
			name: "エクスポートされたポインタ型フィールドでJSONタグが\"-\"の場合はインラインフラグメント",
			args: args{
				field: types.NewField(0, nil, "TestField", types.NewPointer(types.Typ[types.String]), false),
				tag:   `json:"-"`,
			},
			want: want{
				isInlineFragment: true,
			},
		},
		{
			name: "エクスポートされていないフィールドはインラインフラグメントではない",
			args: args{
				field: types.NewField(0, nil, "testField", types.NewPointer(types.Typ[types.String]), false),
				tag:   "",
			},
			want: want{
				isInlineFragment: false,
			},
		},
		{
			name: "JSONタグが指定されている場合はインラインフラグメントではない",
			args: args{
				field: types.NewField(0, nil, "TestField", types.NewPointer(types.Typ[types.String]), false),
				tag:   `json:"test_field"`,
			},
			want: want{
				isInlineFragment: false,
			},
		},
		{
			name: "ポインタ型でない場合はインラインフラグメントではない",
			args: args{
				field: types.NewField(0, nil, "TestField", types.Typ[types.String], false),
				tag:   "",
			},
			want: want{
				isInlineFragment: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := analyzer.IsInlineFragment(tt.args.field, tt.args.tag)

			if diff := cmp.Diff(tt.want.isInlineFragment, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestFieldAnalyzer_IsFragmentSpread(t *testing.T) {
	t.Parallel()

	analyzer := NewFieldAnalyzer()

	type args struct {
		field FieldInfo
	}

	type want struct {
		isFragmentSpread bool
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "埋め込みフィールドでJSONタグが空の場合はフラグメントスプレッド",
			args: args{
				field: FieldInfo{
					IsEmbedded: true,
					JSONTag:    "",
				},
			},
			want: want{
				isFragmentSpread: true,
			},
		},
		{
			name: "埋め込みフィールドでJSONタグが\"-\"の場合はフラグメントスプレッド",
			args: args{
				field: FieldInfo{
					IsEmbedded: true,
					JSONTag:    "-",
				},
			},
			want: want{
				isFragmentSpread: true,
			},
		},
		{
			name: "埋め込みフィールドでない場合はフラグメントスプレッドではない",
			args: args{
				field: FieldInfo{
					IsEmbedded: false,
					JSONTag:    "",
				},
			},
			want: want{
				isFragmentSpread: false,
			},
		},
		{
			name: "埋め込みフィールドでもJSONタグが指定されている場合はフラグメントスプレッドではない",
			args: args{
				field: FieldInfo{
					IsEmbedded: true,
					JSONTag:    "test_field",
				},
			},
			want: want{
				isFragmentSpread: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := analyzer.IsFragmentSpread(tt.args.field)

			if diff := cmp.Diff(tt.want.isFragmentSpread, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestFieldAnalyzer_parseJSONTag(t *testing.T) {
	t.Parallel()

	analyzer := NewFieldAnalyzer()

	type args struct {
		tag string
	}

	type want struct {
		jsonTag string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "JSONタグが指定されている場合はフィールド名を返す",
			args: args{
				tag: `json:"test_field"`,
			},
			want: want{
				jsonTag: "test_field",
			},
		},
		{
			name: "JSONタグにオプションが含まれている場合はフィールド名のみを返す",
			args: args{
				tag: `json:"test_field,omitempty"`,
			},
			want: want{
				jsonTag: "test_field",
			},
		},
		{
			name: "JSONタグが\"-\"の場合は\"-\"を返す",
			args: args{
				tag: `json:"-"`,
			},
			want: want{
				jsonTag: "-",
			},
		},
		{
			name: "JSONタグがない場合は空文字を返す",
			args: args{
				tag: "",
			},
			want: want{
				jsonTag: "",
			},
		},
		{
			name: "JSONタグが空の場合は空文字を返す",
			args: args{
				tag: `json:""`,
			},
			want: want{
				jsonTag: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := analyzer.parseJSONTag(tt.args.tag)

			if diff := cmp.Diff(tt.want.jsonTag, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
