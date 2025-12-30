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
