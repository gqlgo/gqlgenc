package querygen

import (
	"go/types"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/internal/gosrc"
)

func TestGenCode(t *testing.T) {
	t.Parallel()

	// namedType は、パッケージに属さない名前付きの型を作る (生成ファイルと同じパッケージの型にあたる)
	namedType := func(name string) *types.Named {
		return types.NewNamed(types.NewTypeName(0, nil, name, nil), types.NewStruct(nil, nil), nil)
	}

	type args struct {
		responseType    *codegen.ResponseType
		generateGetters bool
	}

	type want struct {
		code string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// go/types に struct を書かせると "json:\"id\"" になり、読みにくく検索もしにくい (gqlgo/gqlgenc#326)
			name: "フラグメントの無い型は、型の宣言だけを書き、タグをraw文字列で書く",
			args: args{
				responseType: &codegen.ResponseType{
					Name: "GetUser_User",
					Fields: codegen.Fields{
						{Name: "id", Type: types.Typ[types.String], TypeKind: codegen.Scalar},
						{Name: "name", Type: types.NewPointer(types.Typ[types.String]), TypeKind: codegen.Scalar},
						{Name: "profile", Type: types.NewPointer(namedType("GetUser_User_Profile")), TypeKind: codegen.Object},
					},
				},
				generateGetters: false,
			},
			want: want{
				code: "type GetUser_User struct {\n" +
					"ID string `json:\"id\"`\n" +
					"Name *string `json:\"name\"`\n" +
					"Profile *GetUser_User_Profile `json:\"profile\"`\n" +
					"}\n",
			},
		},
		{
			// go/types は空の interface を interface{} と書く。any に揃える
			name: "空のinterfaceはanyと書く",
			args: args{
				responseType: &codegen.ResponseType{
					Name: "GetItem_Item",
					Fields: codegen.Fields{
						{Name: "values", Type: types.NewPointer(types.NewSlice(types.NewPointer(types.NewInterfaceType(nil, nil)))), TypeKind: codegen.Scalar},
					},
				},
				generateGetters: false,
			},
			want: want{
				code: "type GetItem_Item struct {\n" +
					"Values *[]*any `json:\"values\"`\n" +
					"}\n",
			},
		},
		{
			// フラグメントスプレッドは json:"-" のフィールドにして、同じ JSON オブジェクトからデコードする
			name: "フラグメントスプレッドを持つ型は、plainとフラグメントの型へデコードする",
			args: args{
				responseType: &codegen.ResponseType{
					Name: "GetUser_User",
					Fields: codegen.Fields{
						{Name: "UserFragment", Type: namedType("UserFragment"), TypeKind: codegen.FragmentSpread},
						{Name: "id", Type: types.Typ[types.String], TypeKind: codegen.Scalar},
					},
				},
				generateGetters: false,
			},
			want: want{
				code: "type GetUser_User struct {\n" +
					"UserFragment UserFragment `json:\"-\"`\n" +
					"ID string `json:\"id\"`\n" +
					"}\n" +
					"func (t *GetUser_User) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n" +
					"data, err := dec.ReadValue()\n" +
					"if err != nil {\n" +
					"return err\n" +
					"}\n" +
					"type plain GetUser_User\n" +
					"if err := json.Unmarshal(data, (*plain)(t)); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"if err := json.Unmarshal(data, &t.UserFragment); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"return nil\n" +
					"}\n" +
					"func (t *GetUser_User) MarshalJSONTo(enc *jsontext.Encoder) error {\n" +
					"merged := map[string]jsontext.Value{}\n" +
					"type plain GetUser_User\n" +
					"if err := client.MergeJSONObject(merged, (*plain)(t)); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"if err := client.MergeJSONObject(merged, &t.UserFragment); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"return json.MarshalEncode(enc, merged, json.Deterministic(true))\n" +
					"}\n",
			},
		},
		{
			// __typename は具体型の名前で返るため、interface の Node は実装する具体型の case でデコードする。
			// Go のフィールド名は HTTPError でも、case は GraphQL の型名 HttpError にする
			name: "インラインフラグメントを持つ型は、__typenameが満たす具体型のcaseで枝をデコードする",
			args: args{
				responseType: &codegen.ResponseType{
					Name: "Files_Files",
					Fields: codegen.Fields{
						{Name: "HttpError", Type: types.NewPointer(namedType("Files_Files_HTTPError")), TypeKind: codegen.InlineFragment, PossibleTypes: []string{"HttpError"}},
						{Name: "Node", Type: types.NewPointer(namedType("Files_Files_Node")), TypeKind: codegen.InlineFragment, PossibleTypes: []string{"Document", "Image"}},
						{Name: "__typename", Type: types.Typ[types.String], TypeKind: codegen.Scalar},
					},
				},
				generateGetters: false,
			},
			want: want{
				code: "type Files_Files struct {\n" +
					"HTTPError *Files_Files_HTTPError `json:\"-\"`\n" +
					"Node *Files_Files_Node `json:\"-\"`\n" +
					"Typename string `json:\"__typename\"`\n" +
					"}\n" +
					"func (t *Files_Files) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n" +
					"data, err := dec.ReadValue()\n" +
					"if err != nil {\n" +
					"return err\n" +
					"}\n" +
					"type plain Files_Files\n" +
					"if err := json.Unmarshal(data, (*plain)(t)); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"switch t.Typename {\n" +
					"case \"HttpError\":\n" +
					"t.HTTPError = &Files_Files_HTTPError{}\n" +
					"if err := json.Unmarshal(data, t.HTTPError); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"case \"Document\", \"Image\":\n" +
					"t.Node = &Files_Files_Node{}\n" +
					"if err := json.Unmarshal(data, t.Node); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"}\n" +
					"return nil\n" +
					"}\n" +
					"func (t *Files_Files) MarshalJSONTo(enc *jsontext.Encoder) error {\n" +
					"merged := map[string]jsontext.Value{}\n" +
					"type plain Files_Files\n" +
					"if err := client.MergeJSONObject(merged, (*plain)(t)); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"if t.HTTPError != nil {\n" +
					"if err := client.MergeJSONObject(merged, t.HTTPError); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"}\n" +
					"if t.Node != nil {\n" +
					"if err := client.MergeJSONObject(merged, t.Node); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"}\n" +
					"return json.MarshalEncode(enc, merged, json.Deterministic(true))\n" +
					"}\n",
			},
		},
		{
			// json/v2 は JSON にできるフィールドの無い struct をエラーにするため、plain を通さない
			name: "フラグメントだけの型は、plainを通さない",
			args: args{
				responseType: &codegen.ResponseType{
					Name: "UserFragment",
					Fields: codegen.Fields{
						{Name: "ProfileFragment", Type: namedType("ProfileFragment"), TypeKind: codegen.FragmentSpread},
					},
				},
				generateGetters: false,
			},
			want: want{
				code: "type UserFragment struct {\n" +
					"ProfileFragment ProfileFragment `json:\"-\"`\n" +
					"}\n" +
					"func (t *UserFragment) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n" +
					"data, err := dec.ReadValue()\n" +
					"if err != nil {\n" +
					"return err\n" +
					"}\n" +
					"if err := json.Unmarshal(data, &t.ProfileFragment); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"return nil\n" +
					"}\n" +
					"func (t *UserFragment) MarshalJSONTo(enc *jsontext.Encoder) error {\n" +
					"merged := map[string]jsontext.Value{}\n" +
					"if err := client.MergeJSONObject(merged, &t.ProfileFragment); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"return json.MarshalEncode(enc, merged, json.Deterministic(true))\n" +
					"}\n",
			},
		},
		{
			// 実装する型の無い interface への型条件は、どの __typename でも値が入らない
			name: "満たす具体型の無いインラインフラグメントだけなら、switchを書かない",
			args: args{
				responseType: &codegen.ResponseType{
					Name: "GetNode_Node",
					Fields: codegen.Fields{
						{Name: "Draft", Type: types.NewPointer(namedType("GetNode_Node_Draft")), TypeKind: codegen.InlineFragment, PossibleTypes: nil},
					},
				},
				generateGetters: false,
			},
			want: want{
				code: "type GetNode_Node struct {\n" +
					"Draft *GetNode_Node_Draft `json:\"-\"`\n" +
					"}\n" +
					"func (t *GetNode_Node) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n" +
					"data, err := dec.ReadValue()\n" +
					"if err != nil {\n" +
					"return err\n" +
					"}\n" +
					"return nil\n" +
					"}\n" +
					"func (t *GetNode_Node) MarshalJSONTo(enc *jsontext.Encoder) error {\n" +
					"merged := map[string]jsontext.Value{}\n" +
					"if t.Draft != nil {\n" +
					"if err := client.MergeJSONObject(merged, t.Draft); err != nil {\n" +
					"return err\n" +
					"}\n" +
					"}\n" +
					"return json.MarshalEncode(enc, merged, json.Deterministic(true))\n" +
					"}\n",
			},
		},
		{
			// getter はフラグメントのフィールドにも生成する
			name: "generateGettersがtrueなら、フィールドごとにgetterを書く",
			args: args{
				responseType: &codegen.ResponseType{
					Name: "GetUser_User",
					Fields: codegen.Fields{
						{Name: "id", Type: types.Typ[types.String], TypeKind: codegen.Scalar},
						{Name: "name", Type: types.NewPointer(types.Typ[types.String]), TypeKind: codegen.Scalar},
					},
				},
				generateGetters: true,
			},
			want: want{
				code: "type GetUser_User struct {\n" +
					"ID string `json:\"id\"`\n" +
					"Name *string `json:\"name\"`\n" +
					"}\n" +
					"func (t *GetUser_User) GetID() string {\n" +
					"\tif t == nil {\n" +
					"\t\tt = &GetUser_User{}\n" +
					"\t}\n" +
					"\treturn t.ID\n" +
					"}\n" +
					"func (t *GetUser_User) GetName() *string {\n" +
					"\tif t == nil {\n" +
					"\t\tt = &GetUser_User{}\n" +
					"\t}\n" +
					"\treturn t.Name\n" +
					"}\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := genCode(tt.args.responseType, tt.args.generateGetters, gosrc.NewImports("example.com/shop/gen"))

			if diff := cmp.Diff(tt.want.code, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestTypenameCases(t *testing.T) {
	t.Parallel()

	// inlineFragment は、型条件 typeCondition を possibleTypes の具体型が満たすインラインフラグメントを作る
	inlineFragment := func(typeCondition string, possibleTypes ...string) *codegen.Field {
		return &codegen.Field{Name: typeCondition, Type: nil, TypeKind: codegen.InlineFragment, PossibleTypes: possibleTypes}
	}

	type args struct {
		inlineFragments []*codegen.Field
	}

	type want struct {
		cases []typenameCase
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// Object 型への型条件は、型条件の名前がそのまま __typename になる
			name: "Object型への型条件は、型ごとに1つのcaseになる",
			args: args{
				inlineFragments: []*codegen.Field{
					inlineFragment("Document", "Document"),
					inlineFragment("Image", "Image"),
				},
			},
			want: want{
				cases: []typenameCase{
					{Typenames: []string{"Document"}, Fragments: []*codegen.Field{inlineFragment("Document", "Document")}},
					{Typenames: []string{"Image"}, Fragments: []*codegen.Field{inlineFragment("Image", "Image")}},
				},
			},
		},
		{
			// __typename は具体型の名前なので、interface の名前の case では一致しない (gqlgo/gqlgenc#325)。
			// 同じ枝の組み合わせをデコードする具体型は 1 つの case にまとめる
			name: "interfaceへの型条件は、実装する具体型を1つのcaseにまとめる",
			args: args{
				inlineFragments: []*codegen.Field{
					inlineFragment("Node", "Document", "Image"),
				},
			},
			want: want{
				cases: []typenameCase{
					{Typenames: []string{"Document", "Image"}, Fragments: []*codegen.Field{inlineFragment("Node", "Document", "Image")}},
				},
			},
		},
		{
			// Document は Document・File・Node の 3 つの型条件を満たし、Image は File と Node だけを、
			// Item は Node だけを満たす
			name: "1つの具体型が複数の型条件を満たすときは、そのcaseに該当する枝をすべて入れる",
			args: args{
				inlineFragments: []*codegen.Field{
					inlineFragment("Document", "Document"),
					inlineFragment("File", "Document", "Image"),
					inlineFragment("Node", "Document", "Image", "Item"),
				},
			},
			want: want{
				cases: []typenameCase{
					{
						Typenames: []string{"Document"},
						Fragments: []*codegen.Field{
							inlineFragment("Document", "Document"),
							inlineFragment("File", "Document", "Image"),
							inlineFragment("Node", "Document", "Image", "Item"),
						},
					},
					{
						Typenames: []string{"Image"},
						Fragments: []*codegen.Field{
							inlineFragment("File", "Document", "Image"),
							inlineFragment("Node", "Document", "Image", "Item"),
						},
					},
					{
						Typenames: []string{"Item"},
						Fragments: []*codegen.Field{
							inlineFragment("Node", "Document", "Image", "Item"),
						},
					},
				},
			},
		},
		{
			// 実装する型の無い interface への型条件は、どの __typename でも値が入らない
			name: "満たす具体型の無い型条件は、どのcaseにも入らない",
			args: args{
				inlineFragments: []*codegen.Field{
					inlineFragment("Draft"),
					inlineFragment("Image", "Image"),
				},
			},
			want: want{
				cases: []typenameCase{
					{Typenames: []string{"Image"}, Fragments: []*codegen.Field{inlineFragment("Image", "Image")}},
				},
			},
		},
		{
			name: "インラインフラグメントが無ければ、caseを作らない",
			args: args{
				inlineFragments: nil,
			},
			want: want{
				cases: []typenameCase{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := typenameCases(tt.args.inlineFragments)

			if diff := cmp.Diff(tt.want.cases, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
