package querygen

import (
	"fmt"
	"strings"
)

// UnmarshalBuilder は UnmarshalJSONFrom メソッドのステートメントを構築する。
type UnmarshalBuilder struct{}

// NewUnmarshalBuilder は新しい UnmarshalBuilder を作成する。
func NewUnmarshalBuilder() *UnmarshalBuilder {
	return &UnmarshalBuilder{}
}

// NeedsUnmarshalMethod は型に UnmarshalJSONFrom メソッドが必要かを判定する。
//
// fragment spreads (json:"-" の埋め込みフィールド) と inline fragments
// (__typename による型分岐) は json/v2 のデフォルトデコードでは処理できないため、
// それらを含む型にのみメソッドを生成する。通常フィールドのみの型は
// json タグによるデフォルトデコードで十分なため、メソッドを生成しない。
//
// パラメータ:
//   - fields: 型のフィールド情報のリスト
//
// 戻り値:
//   - bool: UnmarshalJSONFrom を生成すべき場合は true
func (b *UnmarshalBuilder) NeedsUnmarshalMethod(fields []FieldInfo) bool {
	_, fragmentSpreads, inlineFragments := b.separateFieldTypesList(fields)
	return len(fragmentSpreads) > 0 || len(inlineFragments) > 0
}

// BuildUnmarshalMethod は完全な UnmarshalJSONFrom メソッド本体を構築する。
//
// 生成されるメソッドは値全体を一度だけバッファし、次の順でデコードする:
//  1. 通常フィールド: メソッドを持たない別名型 (type plain T) を経由して
//     json/v2 のデフォルトデコードに任せる
//  2. fragment spreads: json:"-" のため 1 では処理されないので、
//     同じデータから各埋め込みフィールドへデコードする
//  3. inline fragments: __typename の値で分岐してデコードする
//
// パラメータ:
//   - typeName: レシーバ型の名前（例: "UserOperation_User"）
//   - fields: 型のフィールド情報のリスト
//
// 戻り値:
//   - []Statement: UnmarshalJSONFrom メソッド本体のステートメントリスト
func (b *UnmarshalBuilder) BuildUnmarshalMethod(typeName string, fields []FieldInfo) []Statement {
	regularFields, fragmentSpreads, inlineFragments := b.separateFieldTypesList(fields)

	var statements []Statement

	// 1. 値全体を一度だけバッファする。
	statements = append(statements,
		&RawStatement{Code: "data, err := dec.ReadValue()"},
		&IfStatement{
			Condition: "err != nil",
			Body:      []Statement{&ReturnStatement{Value: "err"}},
		},
	)

	// 2. 通常フィールドをデフォルトデコードに任せる。
	if hasDecodableField(regularFields) {
		statements = append(statements,
			&RawStatement{Code: fmt.Sprintf("type plain %s", typeName)},
			&ErrorCheckStatement{
				ErrorExpr: "json.Unmarshal(data, (*plain)(t))",
				Body:      []Statement{&ReturnStatement{Value: "err"}},
			},
		)
	}

	// 3. Decode fragment spreads (non-pointer embedded fields with json:"-").
	statements = append(statements, b.decodeFragmentSpreadsAt(fragmentSpreads, "t")...)

	// 4. Decode inline fragments (__typename based).
	statements = append(statements, b.decodeInlineFragments("t", inlineFragments)...)

	// 5. Return nil on success.
	statements = append(statements, &ReturnStatement{Value: "nil"})

	return statements
}

// decodeFragmentSpreadsAt はカスタム親パスで fragment spreads をデコードするステートメントを生成する。
//
// パラメータ:
//   - fragmentSpreads: fragment spread フィールドのリスト
//   - parentPath: 親パス（例: "t", "t.UserFragment"）
//
// 戻り値:
//   - []Statement: 全ての fragment spreads をデコードするステートメントのリスト
func (b *UnmarshalBuilder) decodeFragmentSpreadsAt(fragmentSpreads []FieldInfo, parentPath string) []Statement {
	var statements []Statement
	for _, field := range fragmentSpreads {
		statements = append(statements, b.decodeSingleFragmentSpread(field, parentPath)...)
	}
	return statements
}

// decodeSingleFragmentSpread は単一の fragment spread フィールドをデコードするステートメントを生成する。
//
// フラグメント型はメソッドを持たないため、json.Unmarshal で同じデータから
// デフォルトデコードする。フラグメント内の json:"-" フィールド
// (ネストした fragment spreads と inline fragments) はそれでは処理されないため、
// 再帰的に明示デコードする。
//
// デコード可能な通常フィールドを持たないフラグメント (ネストしたフラグメントのみで
// 構成される場合) は、json/v2 が「JSON 表現可能なフィールドのない構造体」を
// エラーにするため、直接デコードをスキップして再帰処理のみ行う。
//
// パラメータ:
//   - field: fragment spread フィールドの情報
//   - parentPath: 親パス（例: "t", "t.UserFragment"）
//
// 戻り値:
//   - []Statement: このフィールドをデコードするステートメントのリスト
func (b *UnmarshalBuilder) decodeSingleFragmentSpread(field FieldInfo, parentPath string) []Statement {
	target := fmt.Sprintf("%s.%s", parentPath, field.Name)
	regularFields, fragmentSpreads, inlineFragments := b.separateFieldTypesAt(field.SubFields, target)

	var statements []Statement

	// SubFields が解析できない型（独自の UnmarshalJSONFrom を持つ埋め込み型など）も
	// 値全体からのデコードに任せる。
	if len(field.SubFields) == 0 || hasDecodableField(regularFields) {
		statements = append(statements, &ErrorCheckStatement{
			ErrorExpr: fmt.Sprintf("json.Unmarshal(data, &%s)", target),
			Body: []Statement{
				&ReturnStatement{Value: "err"},
			},
		})
	}

	statements = append(statements, b.decodeFragmentSpreadsAt(fragmentSpreads, target)...)
	statements = append(statements, b.decodeInlineFragments(target, inlineFragments)...)

	return statements
}

// hasDecodableField は json タグによるデフォルトデコードの対象になる
// フィールドが1つでもあるかを返す。
func hasDecodableField(fields []FieldInfo) bool {
	for _, field := range fields {
		if field.JSONTag == "" || field.JSONTag == "-" {
			continue
		}
		if !field.IsExported {
			continue
		}
		return true
	}
	return false
}

// separateFieldTypesList はデコード戦略によってフィールドのリストを分類する。
//
// デフォルトパス "t" で separateFieldTypesAt に委譲する。
//
// パラメータ:
//   - fields: 分類対象のフィールドリスト
//
// 戻り値:
//   - []FieldInfo: 通常フィールドのリスト
//   - []FieldInfo: fragment spreads のリスト
//   - []InlineFragmentInfo: inline fragments のリスト
func (b *UnmarshalBuilder) separateFieldTypesList(fields []FieldInfo) ([]FieldInfo, []FieldInfo, []InlineFragmentInfo) {
	return b.separateFieldTypesAt(fields, "t")
}

// separateFieldTypesAt はカスタム親パスでフィールドのリストを分類する。
//
// このメソッドはトップレベルフィールドとネストフィールド（埋め込み構造体）の
// 両方に使用される。parentPath パラメータは inline fragments のフィールド式を
// 構築する際に使用するターゲット式（例: "t" または "t.NestedField"）を指定する。
//
// パラメータ:
//   - fields: 分類対象のフィールドリスト
//   - parentPath: 親パス（例: "t", "t.NestedField"）
//
// 戻り値:
//   - []FieldInfo: 通常フィールド（JSON タグを持つ）
//   - []FieldInfo: Fragment spreads（json:"-" を持つ埋め込みフィールド）
//   - []InlineFragmentInfo: Inline fragments（型条件付きポインタフィールド）
func (b *UnmarshalBuilder) separateFieldTypesAt(fields []FieldInfo, parentPath string) ([]FieldInfo, []FieldInfo, []InlineFragmentInfo) {
	var regularFields []FieldInfo
	var fragmentSpreads []FieldInfo
	var inlineFragments []InlineFragmentInfo

	for _, field := range fields {
		switch {
		case field.IsInlineFragment:
			inlineFragments = append(inlineFragments, InlineFragmentInfo{
				Field:       field,
				FieldExpr:   fmt.Sprintf("%s.%s", parentPath, field.Name),
				ElemTypeStr: field.PointerElemType,
			})
		case field.IsEmbedded && (field.JSONTag == "" || field.JSONTag == "-"):
			fragmentSpreads = append(fragmentSpreads, field)
		default:
			regularFields = append(regularFields, field)
		}
	}

	return regularFields, fragmentSpreads, inlineFragments
}

// decodeInlineFragments は __typename を使って inline fragments をデコードするステートメントを生成する。
//
// 以下のようなコードを生成する:
//
//	var typeName_t struct {
//	    Typename string `json:"__typename"`
//	}
//	if err := json.Unmarshal(data, &typeName_t); err != nil {
//	    return err
//	}
//	switch typeName_t.Typename {
//	case "User":
//	    t.User = &UserFragment{}
//	    if err := json.Unmarshal(data, t.User); err != nil {
//	        return err
//	    }
//	}
//
// パラメータ:
//   - targetExpr: ターゲット構造体の式（例: "t"）
//   - fragments: デコードする inline fragment フィールド
//
// 戻り値:
//   - []Statement: inline fragments をデコードするステートメントのリスト（空の場合は nil）
func (b *UnmarshalBuilder) decodeInlineFragments(targetExpr string, fragments []InlineFragmentInfo) []Statement {
	if len(fragments) == 0 {
		return nil
	}

	typeNameVar := fmt.Sprintf("typeName_%s", strings.ReplaceAll(targetExpr, ".", "_"))

	var statements []Statement

	statements = append(statements, &RawStatement{
		Code: fmt.Sprintf("var %s struct {\n\t\tTypename string `json:\"__typename\"`\n\t}", typeNameVar),
	})
	statements = append(statements, &ErrorCheckStatement{
		ErrorExpr: fmt.Sprintf("json.Unmarshal(data, &%s)", typeNameVar),
		Body: []Statement{
			&ReturnStatement{Value: "err"},
		},
	})

	cases := make([]SwitchCase, 0, len(fragments))
	for _, frag := range fragments {
		cases = append(cases, SwitchCase{
			Value: frag.Field.Name,
			Body:  b.buildInlineFragmentCaseBody(frag),
		})
	}

	statements = append(statements, &SwitchStatement{
		Expr:  typeNameVar + ".Typename",
		Cases: cases,
	})

	return statements
}

// buildInlineFragmentCaseBody は inline fragment の switch case 本体を構築する。
//
// ポインタフィールドを初期化し、デフォルトデコードでフィールドを充填する。
// inline fragment の構造体内の fragment spreads (json:"-") はデフォルトデコードでは
// 処理されないため、再帰的に明示デコードする。
//
// デコード可能な通常フィールドを持たない構造体 (フラグメントのみで構成される場合) は、
// json/v2 が「JSON 表現可能なフィールドのない構造体」をエラーにするため、
// 直接デコードをスキップして再帰処理のみ行う。
//
// パラメータ:
//   - frag: inline fragment の情報
//
// 戻り値:
//   - []Statement: case 本体のステートメントリスト
func (b *UnmarshalBuilder) buildInlineFragmentCaseBody(frag InlineFragmentInfo) []Statement {
	statements := []Statement{
		&Assignment{
			Target: frag.FieldExpr,
			Value:  fmt.Sprintf("&%s{}", frag.ElemTypeStr),
		},
	}

	regularFields, fragmentSpreads, inlineFragments := b.separateFieldTypesAt(frag.Field.SubFields, frag.FieldExpr)

	if len(frag.Field.SubFields) == 0 || hasDecodableField(regularFields) {
		statements = append(statements, &ErrorCheckStatement{
			ErrorExpr: fmt.Sprintf("json.Unmarshal(data, %s)", frag.FieldExpr),
			Body: []Statement{
				&ReturnStatement{Value: "err"},
			},
		})
	}

	statements = append(statements, b.decodeFragmentSpreadsAt(fragmentSpreads, frag.FieldExpr)...)
	statements = append(statements, b.decodeInlineFragments(frag.FieldExpr, inlineFragments)...)

	return statements
}
