package querygen

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/99designs/gqlgen/codegen/templates"

	"github.com/Yamashou/gqlgenc/v3/codegen"
)

// genCode は、レスポンスの型 1 つのコードを返す。型の宣言に続けて、フラグメントを持つ型には
// UnmarshalJSONFrom と MarshalJSONTo を、generateGetters が true ならフィールドごとの getter を書く。
// 整形は書き出すときの gofmt に任せ、インデントしない。
func genCode(responseType *codegen.ResponseType, generateGetters bool) string {
	var buf strings.Builder

	writeTypeDecl(&buf, responseType)

	// フラグメントのフィールドは JSON のメンバーに対応しないため、既定のデコードでは値が入らない。
	// MarshalJSONTo は、JSON にして戻す往復でフラグメントのフィールドを落とさないよう、対で生成する。
	if hasFragment(responseType) {
		writeUnmarshalMethod(&buf, responseType)
		writeMarshalMethod(&buf, responseType)
	}

	if generateGetters {
		for _, field := range responseType.Fields {
			writeGetter(&buf, responseType.Name, field)
		}
	}

	return buf.String()
}

// hasFragment は、型がフラグメントスプレッドかインラインフラグメントのフィールドを持つかを返す。
func hasFragment(responseType *codegen.ResponseType) bool {
	return slices.ContainsFunc(responseType.Fields, (*codegen.Field).IsFragment)
}

// hasDecodableField は、既定のデコードで値が入るフィールド (フラグメントでないフィールド) を型が
// 持つかを返す。json/v2 は JSON にできるフィールドの無い struct をエラーにするため、無い型は
// 既定のデコードを通さない。別名 _ のフィールドは Go の名前も _ で、JSON にできないため数えない。
func hasDecodableField(responseType *codegen.ResponseType) bool {
	return slices.ContainsFunc(responseType.Fields, func(field *codegen.Field) bool {
		return !field.IsFragment() && token.IsExported(field.GoName())
	})
}

// fieldsOfKind は、型のフィールドのうち種類が kind のものを、フィールドの順に返す。
func fieldsOfKind(responseType *codegen.ResponseType, kind codegen.TypeKind) []*codegen.Field {
	var fields []*codegen.Field
	for _, field := range responseType.Fields {
		if field.TypeKind == kind {
			fields = append(fields, field)
		}
	}

	return fields
}

// goTypeString は、生成コードに書く型の表記を返す。パッケージ名は生成ファイルの import に合わせて
// 修飾する。go/types は空の interface を interface{} と書くため、any に揃える
// (空でない interface は interface{ M() } の形になるので、置換の対象にならない)。
func goTypeString(t types.Type) string {
	return strings.ReplaceAll(templates.CurrentImports.LookupType(t), "interface{}", "any")
}

// writeTypeDecl は型の宣言を書く。フィールドの JSON のタグは raw 文字列 (`json:"id"`) で書く
// (gqlgo/gqlgenc#326)。フラグメントのフィールドは JSON のメンバーに対応しないため json:"-" にする。
func writeTypeDecl(buf *strings.Builder, responseType *codegen.ResponseType) {
	fmt.Fprintf(buf, "type %s struct {\n", responseType.Name)
	for _, field := range responseType.Fields {
		jsonName := field.Name
		if field.IsFragment() {
			jsonName = "-"
		}
		fmt.Fprintf(buf, "%s %s `json:\"%s\"`\n", field.GoName(), goTypeString(field.Type), jsonName)
	}
	buf.WriteString("}\n")
}

// writeUnmarshalMethod は UnmarshalJSONFrom を書く。
//
// 値全体を 1 度だけ読み、同じ JSON オブジェクトから次の順にデコードする。
//  1. フラグメントでないフィールド。メソッドを持たない別名の型 plain を通して、既定のデコードに任せる
//  2. フラグメントスプレッド。フラグメントの型へそのままデコードする
//  3. インラインフラグメント。__typename が型条件を満たす具体型のときだけ、枝の型を確保してデコードする
//
// フラグメントの型がさらにフラグメントを持つときは、その型の UnmarshalJSONFrom がデコードする。
func writeUnmarshalMethod(buf *strings.Builder, responseType *codegen.ResponseType) {
	fmt.Fprintf(buf, "func (t *%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n", responseType.Name)
	buf.WriteString("data, err := dec.ReadValue()\nif err != nil {\nreturn err\n}\n")

	if hasDecodableField(responseType) {
		fmt.Fprintf(buf, "type plain %s\n", responseType.Name)
		buf.WriteString("if err := json.Unmarshal(data, (*plain)(t)); err != nil {\nreturn err\n}\n")
	}

	for _, field := range fieldsOfKind(responseType, codegen.FragmentSpread) {
		fmt.Fprintf(buf, "if err := json.Unmarshal(data, &t.%s); err != nil {\nreturn err\n}\n", field.GoName())
	}

	// __typename は非 null の string として生成するため、ポインタを外さずに分岐する
	if cases := typenameCases(fieldsOfKind(responseType, codegen.InlineFragment)); len(cases) > 0 {
		buf.WriteString("switch t.Typename {\n")
		for _, typenameCase := range cases {
			quoted := make([]string, 0, len(typenameCase.Typenames))
			for _, typename := range typenameCase.Typenames {
				quoted = append(quoted, strconv.Quote(typename))
			}
			fmt.Fprintf(buf, "case %s:\n", strings.Join(quoted, ", "))

			for _, field := range typenameCase.Fragments {
				// インラインフラグメントのフィールドの型は、枝の型へのポインタ
				branchType := strings.TrimPrefix(goTypeString(field.Type), "*")
				fmt.Fprintf(buf, "t.%s = &%s{}\n", field.GoName(), branchType)
				fmt.Fprintf(buf, "if err := json.Unmarshal(data, t.%s); err != nil {\nreturn err\n}\n", field.GoName())
			}
		}
		buf.WriteString("}\n")
	}

	buf.WriteString("return nil\n}\n")
}

// typenameCase は、UnmarshalJSONFrom が __typename で分岐する switch の case 1 つを表す。
type typenameCase struct {
	// Typenames は、case に並べる __typename の値 (GraphQL の具体型の名前)
	Typenames []string
	// Fragments は、その具体型のときに値を入れるインラインフラグメントのフィールド
	Fragments []*codegen.Field
}

// typenameCases は、インラインフラグメントのフィールドから、__typename の switch の case を作る。
//
// __typename はレスポンスの具体型 (Object 型) の名前なので、case には型条件の名前ではなく、
// 型条件を満たす具体型の名前を並べる (gqlgo/gqlgenc#325)。interface / union への型条件 (... on Node) は、
// それを満たすすべての具体型の case でデコードする。1 つの具体型が複数の型条件を満たすときは、
// その case で該当するインラインフラグメントをすべてデコードする。同じ組み合わせをデコードする
// 具体型は 1 つの case にまとめる。
//
// 例えば Node を User と Post が実装し、選択が `... on Node` と `... on User` なら、
// case "Post" は Node を、case "User" は Node と User をデコードする。
//
// 満たす具体型が 1 つも無い型条件 (実装する型の無い interface など) は、どの case にも入らない。
func typenameCases(inlineFragments []*codegen.Field) []typenameCase {
	// 具体型ごとに、その型のときに値を入れるインラインフラグメントを集める。具体型は最初に現れた順に並べる
	typenames := make([]string, 0, len(inlineFragments))
	fragmentsByTypename := make(map[string][]*codegen.Field, len(inlineFragments))
	for _, fragment := range inlineFragments {
		for _, typename := range fragment.PossibleTypes {
			if _, ok := fragmentsByTypename[typename]; !ok {
				typenames = append(typenames, typename)
			}
			fragmentsByTypename[typename] = append(fragmentsByTypename[typename], fragment)
		}
	}

	cases := make([]typenameCase, 0, len(typenames))
	caseIndexes := make(map[string]int, len(typenames))
	for _, typename := range typenames {
		fragments := fragmentsByTypename[typename]

		names := make([]string, 0, len(fragments))
		for _, fragment := range fragments {
			names = append(names, fragment.Name)
		}
		key := strings.Join(names, ",")

		if i, ok := caseIndexes[key]; ok {
			cases[i].Typenames = append(cases[i].Typenames, typename)
			continue
		}

		caseIndexes[key] = len(cases)
		cases = append(cases, typenameCase{Typenames: []string{typename}, Fragments: fragments})
	}

	return cases
}

// writeMarshalMethod は MarshalJSONTo を書く。
//
// UnmarshalJSONFrom と対称になるよう、フラグメントでないフィールド (plain を通す)・フラグメント
// スプレッド・値の入ったインラインフラグメントを 1 つの JSON オブジェクトへまとめて出力する。
// 出力のメンバーは、キーの昇順に並べる。json/v2 は既定では map のメンバーを反復の順 (実行のたびに
// 変わる) で出力するため、json.Deterministic を指定して、同じ値から毎回同じ JSON を得る。
func writeMarshalMethod(buf *strings.Builder, responseType *codegen.ResponseType) {
	fmt.Fprintf(buf, "func (t *%s) MarshalJSONTo(enc *jsontext.Encoder) error {\n", responseType.Name)
	buf.WriteString("merged := map[string]jsontext.Value{}\n")

	if hasDecodableField(responseType) {
		fmt.Fprintf(buf, "type plain %s\n", responseType.Name)
		buf.WriteString("if err := client.MergeJSONObject(merged, (*plain)(t)); err != nil {\nreturn err\n}\n")
	}

	for _, field := range fieldsOfKind(responseType, codegen.FragmentSpread) {
		fmt.Fprintf(buf, "if err := client.MergeJSONObject(merged, &t.%s); err != nil {\nreturn err\n}\n", field.GoName())
	}

	for _, field := range fieldsOfKind(responseType, codegen.InlineFragment) {
		fmt.Fprintf(buf, "if t.%s != nil {\nif err := client.MergeJSONObject(merged, t.%s); err != nil {\nreturn err\n}\n}\n", field.GoName(), field.GoName())
	}

	buf.WriteString("return json.MarshalEncode(enc, merged, json.Deterministic(true))\n}\n")
}

// writeGetter は、フィールドの getter を書く。getter はレシーバが nil でも呼べて、そのときは
// フィールドのゼロ値を返す。
func writeGetter(buf *strings.Builder, typeName string, field *codegen.Field) {
	fmt.Fprintf(buf, `func (t *%s) Get%s() %s {
	if t == nil {
		t = &%s{}
	}
	return t.%s
}
`, typeName, field.GoName(), goTypeString(field.Type), typeName, field.GoName())
}
