package querygen

import (
	"go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFieldClassifier_IsInlineFragment(t *testing.T) {
	t.Parallel()

	classifier := NewFieldClassifier()

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

			got := classifier.IsInlineFragment(tt.args.field, tt.args.tag)

			if diff := cmp.Diff(tt.want.isInlineFragment, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestFieldClassifier_IsFragmentSpread(t *testing.T) {
	t.Parallel()

	classifier := NewFieldClassifier()

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

			got := classifier.IsFragmentSpread(tt.args.field)

			if diff := cmp.Diff(tt.want.isFragmentSpread, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestFieldClassifier_parseJSONTag(t *testing.T) {
	t.Parallel()

	classifier := NewFieldClassifier()

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

			got := classifier.parseJSONTag(tt.args.tag)

			if diff := cmp.Diff(tt.want.jsonTag, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
