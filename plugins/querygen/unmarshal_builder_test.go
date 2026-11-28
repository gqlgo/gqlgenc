package querygen

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnmarshalBuilder_decodeFragmentSpreadsAt(t *testing.T) {
	type args struct {
		fragmentSpreads []FieldInfo
		parentPath      string
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
				parentPath:      "t",
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
				parentPath: "t",
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
				parentPath: "t",
			},
			want: want{
				// 親はデコード可能な通常フィールドを持たないため直接デコードはスキップされ、
				// 子のデコード statement のみ = 1
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
				parentPath: "t",
			},
			want: want{
				statementsCount: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewUnmarshalBuilder()
			got := b.decodeFragmentSpreadsAt(tt.args.fragmentSpreads, tt.args.parentPath)

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
		notContains     []string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// SubFields が解析できない場合は data 全体のデコードに任せる
			name: "SubFieldsがない場合はdata全体をデコードする",
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
			// 通常フィールドを持つフラグメントは data 全体のデフォルトデコード1文になる
			name: "通常のSubFieldsを持つ場合もdata全体のデコード1文になる",
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
				statementsCount: 1,
				contains: []string{
					"json.Unmarshal(data, &t.UserFragment)",
				},
				notContains: []string{
					"raw[",
				},
			},
		},
		{
			// 通常フィールドがなくネストしたフラグメントのみの場合、
			// 直接デコードはスキップされ子の処理のみが生成される
			name: "ネストしたfragmentのみの場合は子の処理のみ生成される",
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
				notContains: []string{
					"json.Unmarshal(data, &t.UserFragment)\n",
				},
			},
		},
		{
			// inline fragment は __typename ホルダー構造体経由の switch でデコードされる
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
				// __typename は非 null string なので switch のみ = 1
				statementsCount: 1,
				contains: []string{
					"switch t.UserFragment.Typename {",
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
				buf.WriteString(stmt.String())
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
		})
	}
}

func TestUnmarshalBuilder_BuildUnmarshalMethod(t *testing.T) {
	t.Parallel()

	type args struct {
		typeName string
		fields   []FieldInfo
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
			// 通常フィールドは type plain 経由のデフォルトデコードに任せる
			name: "fragment spreadを含む型はplainデコードとfragmentデコードを生成する",
			args: args{
				typeName: "UserOperation",
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
					"type plain UserOperation",
					"json.Unmarshal(data, (*plain)(t))",
					"json.Unmarshal(data, &t.UserFragment)",
				},
				notContains: []string{
					"map[string]jsontext.Value",
					"raw[",
				},
			},
		},
		{
			// inline fragment は __typename ホルダー構造体で分岐する
			name: "inline fragmentを含む型は__typename分岐を生成する",
			args: args{
				typeName: "UserOperation_Node",
				fields: []FieldInfo{
					{
						Name:       "Typename",
						JSONTag:    "__typename",
						IsExported: true,
					},
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
					"type plain UserOperation_Node",
					"switch t.Typename {",
					"t.User = &UserFragment{}",
					"json.Unmarshal(data, t.User)",
				},
			},
		},
		{
			// デコード可能な通常フィールドが無い型では plain デコードを生成しない
			// (JSON 表現可能なフィールドのない構造体のデコードはエラーになるため)
			name: "通常フィールドがない型ではplainデコードを生成しない",
			args: args{
				typeName: "UserOperation_Node",
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
					"switch t.Typename {",
				},
				notContains: []string{
					"(*plain)(t)",
				},
			},
		},
		{
			// インラインフラグメントの構造体内のフラグメントスプレッド (json:"-") は
			// デフォルトデコードでは充填されないため、明示デコードが生成されることを確認する。
			// 旧実装では埋め込みフラグメントのメソッド昇格がデコード全体を乗っ取り、
			// 兄弟フィールドが常にゼロ値になるバグがあった
			name: "inline fragment内のfragment spreadも明示デコードされる",
			args: args{
				typeName: "UserOperation_User",
				fields: []FieldInfo{
					{
						Name:       "Typename",
						JSONTag:    "__typename",
						IsExported: true,
					},
					{
						Name:             "User",
						IsInlineFragment: true,
						IsPointer:        true,
						PointerElemType:  "UserInline",
						SubFields: []FieldInfo{
							{
								Name:       "Name",
								JSONTag:    "name",
								IsExported: true,
							},
							{
								Name:       "UserFragment2",
								IsEmbedded: true,
								JSONTag:    "-",
								SubFields: []FieldInfo{
									{
										Name:       "Name",
										JSONTag:    "name",
										IsExported: true,
									},
								},
							},
						},
					},
				},
			},
			want: want{
				contains: []string{
					"json.Unmarshal(data, &t.User)",
					"json.Unmarshal(data, &t.User.UserFragment2)",
				},
			},
		},
		{
			// デコード可能な通常フィールドを持たないインラインフラグメント構造体では
			// 直接デコードがスキップされ、フラグメントの明示デコードのみ生成される
			name: "fragment spreadのみのinline fragmentでは直接デコードを生成しない",
			args: args{
				typeName: "UserOperation_User",
				fields: []FieldInfo{
					{
						Name:       "Typename",
						JSONTag:    "__typename",
						IsExported: true,
					},
					{
						Name:             "User",
						IsInlineFragment: true,
						IsPointer:        true,
						PointerElemType:  "UserInline",
						SubFields: []FieldInfo{
							{
								Name:       "UserFragment2",
								IsEmbedded: true,
								JSONTag:    "-",
								SubFields: []FieldInfo{
									{
										Name:       "Name",
										JSONTag:    "name",
										IsExported: true,
									},
								},
							},
						},
					},
				},
			},
			want: want{
				contains: []string{
					"t.User = &UserInline{}",
					"json.Unmarshal(data, &t.User.UserFragment2)",
				},
				notContains: []string{
					"json.Unmarshal(data, t.User)",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.BuildUnmarshalMethod(tt.args.typeName, tt.args.fields)

			var buf strings.Builder
			for _, stmt := range got {
				buf.WriteString(stmt.String())
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

func TestUnmarshalBuilder_decodeInlineFragments(t *testing.T) {
	t.Parallel()

	// inlineFragment は、サブフィールドを解析していない (自前の UnmarshalJSONFrom を持つ) 枝を作る
	inlineFragment := func(name string, possibleTypes ...string) InlineFragmentInfo {
		return InlineFragmentInfo{
			Field: FieldInfo{
				Name:             name,
				JSONTag:          "-",
				IsExported:       true,
				IsInlineFragment: true,
				IsPointer:        true,
				PointerElemType:  "Files_" + name,
				PossibleTypes:    possibleTypes,
			},
			FieldExpr:   "t." + name,
			ElemTypeStr: "Files_" + name,
		}
	}

	type args struct {
		fragments  []InlineFragmentInfo
		targetExpr string
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
			// Object 型への型条件は、型条件の名前がそのまま __typename になる
			name: "Object型への型条件は、型ごとに1つのcaseになる",
			args: args{
				fragments: []InlineFragmentInfo{
					inlineFragment("Document", "Document"),
					inlineFragment("Image", "Image"),
				},
				targetExpr: "t",
			},
			want: want{
				code: `switch t.Typename {
case "Document":
t.Document = &Files_Document{}
if err := json.Unmarshal(data, t.Document); err != nil {
return err
}
case "Image":
t.Image = &Files_Image{}
if err := json.Unmarshal(data, t.Image); err != nil {
return err
}
}
`,
			},
		},
		{
			// __typename は具体型の名前なので、interface の名前の case では一致しない (gqlgo/gqlgenc#325)。
			// 同じ枝の組み合わせをデコードする具体型は 1 つの case にまとめる
			name: "interfaceへの型条件は、実装する具体型のcaseでデコードする",
			args: args{
				fragments: []InlineFragmentInfo{
					inlineFragment("Node", "Document", "Image"),
				},
				targetExpr: "t",
			},
			want: want{
				code: `switch t.Typename {
case "Document", "Image":
t.Node = &Files_Node{}
if err := json.Unmarshal(data, t.Node); err != nil {
return err
}
}
`,
			},
		},
		{
			// Document は Document・File・Node の 3 つの型条件を満たし、Image は File と Node だけを満たす
			name: "1つの具体型が複数の型条件を満たすときは、そのcaseで該当する枝をすべてデコードする",
			args: args{
				fragments: []InlineFragmentInfo{
					inlineFragment("Document", "Document"),
					inlineFragment("File", "Document", "Image"),
					inlineFragment("Node", "Document", "Image", "Item"),
				},
				targetExpr: "t",
			},
			want: want{
				code: `switch t.Typename {
case "Document":
t.Document = &Files_Document{}
if err := json.Unmarshal(data, t.Document); err != nil {
return err
}
t.File = &Files_File{}
if err := json.Unmarshal(data, t.File); err != nil {
return err
}
t.Node = &Files_Node{}
if err := json.Unmarshal(data, t.Node); err != nil {
return err
}
case "Image":
t.File = &Files_File{}
if err := json.Unmarshal(data, t.File); err != nil {
return err
}
t.Node = &Files_Node{}
if err := json.Unmarshal(data, t.Node); err != nil {
return err
}
case "Item":
t.Node = &Files_Node{}
if err := json.Unmarshal(data, t.Node); err != nil {
return err
}
}
`,
			},
		},
		{
			// Go のフィールド名は HTTPError でも、__typename は GraphQL の型名 HttpError で返る
			name: "Goの名前と綴りが違う型は、GraphQLの型名でcaseを作る",
			args: args{
				fragments: []InlineFragmentInfo{
					inlineFragment("HTTPError", "HttpError"),
				},
				targetExpr: "t",
			},
			want: want{
				code: `switch t.Typename {
case "HttpError":
t.HTTPError = &Files_HTTPError{}
if err := json.Unmarshal(data, t.HTTPError); err != nil {
return err
}
}
`,
			},
		},
		{
			// codegen を通さずに組み立てた型には内部タグが無いので、フィールド名を型名とみなす
			name: "具体型が渡されていなければ、フィールド名をcaseにする",
			args: args{
				fragments: []InlineFragmentInfo{
					inlineFragment("User"),
				},
				targetExpr: "t",
			},
			want: want{
				code: `switch t.Typename {
case "User":
t.User = &Files_User{}
if err := json.Unmarshal(data, t.User); err != nil {
return err
}
}
`,
			},
		},
		{
			// inline fragment が無い型には switch を生成しない
			name: "inline fragmentが無ければ何も生成しない",
			args: args{
				fragments:  nil,
				targetExpr: "t",
			},
			want: want{
				code: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.decodeInlineFragments(tt.args.fragments, tt.args.targetExpr)

			var buf strings.Builder
			for _, stmt := range got {
				buf.WriteString(stmt.String())
				buf.WriteString("\n")
			}

			if diff := cmp.Diff(tt.want.code, buf.String()); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestUnmarshalBuilder_NeedsUnmarshalMethod(t *testing.T) {
	t.Parallel()

	type args struct {
		fields []FieldInfo
	}

	type want struct {
		needs bool
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 通常フィールドのみの型は json/v2 のデフォルトデコードで十分
			name: "通常フィールドのみの型はメソッド不要",
			args: args{
				fields: []FieldInfo{
					{
						Name:       "ID",
						JSONTag:    "id",
						IsExported: true,
					},
				},
			},
			want: want{
				needs: false,
			},
		},
		{
			name: "fragment spreadを含む型はメソッドが必要",
			args: args{
				fields: []FieldInfo{
					{
						Name:       "UserFragment",
						IsEmbedded: true,
						JSONTag:    "-",
					},
				},
			},
			want: want{
				needs: true,
			},
		},
		{
			name: "inline fragmentを含む型はメソッドが必要",
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
				needs: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewUnmarshalBuilder()
			got := b.NeedsUnmarshalMethod(tt.args.fields)

			if diff := cmp.Diff(tt.want.needs, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
