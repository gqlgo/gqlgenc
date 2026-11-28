package querygen

import (
	"go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCodeGenerator_formatTypeDecl(t *testing.T) {
	t.Parallel()

	// namedStruct は、パッケージに属さない名前付きの struct 型を作る (生成ファイルと同じパッケージの型にあたる)
	namedStruct := func(name string) *types.Named {
		return types.NewNamed(types.NewTypeName(0, nil, name, nil), types.NewStruct(nil, nil), nil)
	}

	type args struct {
		typeName   string
		structType *types.Struct
	}

	type want struct {
		decl string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// go/types に struct を書かせると "json:\"id\"" になり、読みにくく検索もしにくい (gqlgo/gqlgenc#326)
			name: "タグをraw文字列で書く",
			args: args{
				typeName: "GetUser_User",
				structType: types.NewStruct(
					[]*types.Var{
						types.NewField(0, nil, "ID", types.Typ[types.String], false),
						types.NewField(0, nil, "Name", types.NewPointer(types.Typ[types.String]), false),
					},
					[]string{
						`json:"id"`,
						`json:"name"`,
					},
				),
			},
			want: want{
				decl: "type GetUser_User struct {\n" +
					"ID string `json:\"id\"`\n" +
					"Name *string `json:\"name\"`\n" +
					"}\n",
			},
		},
		{
			// 埋め込みフィールド (fragment spread を埋め込む形) は、名前を書かずに型だけを書く
			name: "埋め込みフィールドは型だけを書く",
			args: args{
				typeName: "GetUser_User",
				structType: types.NewStruct(
					[]*types.Var{
						types.NewField(0, nil, "UserFragment", namedStruct("UserFragment"), true),
					},
					[]string{
						`json:"-"`,
					},
				),
			},
			want: want{
				decl: "type GetUser_User struct {\n" +
					"UserFragment `json:\"-\"`\n" +
					"}\n",
			},
		},
		{
			// go/types は空の interface を interface{} と書く。getter の戻り値と同じ any に揃える
			name: "空のinterfaceはanyと書く",
			args: args{
				typeName: "GetItem_Item",
				structType: types.NewStruct(
					[]*types.Var{
						types.NewField(0, nil, "Values", types.NewPointer(types.NewSlice(types.NewPointer(types.NewInterfaceType(nil, nil)))), false),
					},
					[]string{
						`json:"values"`,
					},
				),
			},
			want: want{
				decl: "type GetItem_Item struct {\n" +
					"Values *[]*any `json:\"values\"`\n" +
					"}\n",
			},
		},
		{
			// バッククォートを含むタグは raw 文字列で書けないので、解釈文字列で書く
			name: "バッククォートを含むタグは解釈文字列で書く",
			args: args{
				typeName: "GetItem_Item",
				structType: types.NewStruct(
					[]*types.Var{
						types.NewField(0, nil, "Name", types.Typ[types.String], false),
					},
					[]string{
						"json:\"na`me\"",
					},
				),
			},
			want: want{
				decl: "type GetItem_Item struct {\n" +
					"Name string \"json:\\\"na`me\\\"\"\n" +
					"}\n",
			},
		},
		{
			// タグの無いフィールドと、フィールドの無い struct もそのまま書ける
			name: "タグの無いフィールドにはタグを書かない",
			args: args{
				typeName: "Empty",
				structType: types.NewStruct(
					[]*types.Var{
						types.NewField(0, nil, "Count", types.Typ[types.Int], false),
					},
					nil,
				),
			},
			want: want{
				decl: "type Empty struct {\n" +
					"Count int\n" +
					"}\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := NewCodeGenerator(nil, false)
			got := g.formatTypeDecl(tt.args.typeName, tt.args.structType)

			if diff := cmp.Diff(tt.want.decl, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
