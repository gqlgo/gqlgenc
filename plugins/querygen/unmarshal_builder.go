package querygen

import "fmt"

// UnmarshalBuilder は UnmarshalJSON メソッドのステートメントを構築する。
type UnmarshalBuilder struct {
	fieldDecoder  *FieldDecoder
	inlineDecoder *InlineFragmentDecoder
}

// NewUnmarshalBuilder は新しい UnmarshalBuilder を作成する。
func NewUnmarshalBuilder() *UnmarshalBuilder {
	return &UnmarshalBuilder{
		fieldDecoder:  NewFieldDecoder(),
		inlineDecoder: NewInlineFragmentDecoder(),
	}
}

// BuildUnmarshalMethod は完全な UnmarshalJSON メソッド本体を構築する。
//
// 生成されるメソッドは3種類のフィールドを処理する:
//  1. 通常フィールド: JSON キーからアンマーシャル (json:"fieldName")
//  2. Fragment spreads: json:"-" を持つ埋め込みフィールド（GraphQL fragments）
//  3. Inline fragments: __typename に基づく型条件付きフィールド
//
// メソッドは jsontext.Value (json/v2) を使用して、raw JSON マップでの
// フィールド存在チェック時の不要なアロケーションを回避する。
func (b *UnmarshalBuilder) BuildUnmarshalMethod(typeInfo TypeInfo) []Statement {
	var statements []Statement

	// 1. Declare raw map variable (using jsontext.Value for json/v2).
	statements = append(statements, &VariableDecl{
		Name: "raw",
		Type: "map[string]jsontext.Value",
	})

	// 2. Unmarshal data into raw map.
	statements = append(statements, &ErrorCheckStatement{
		ErrorExpr: "json.Unmarshal(data, &raw)",
		Body: []Statement{
			&ReturnStatement{Value: "err"},
		},
	})

	// 3. Define target and raw expressions for field decoding.
	targetExpr := "t"
	rawExpr := "raw"

	// 4. Separate regular fields, fragment spreads, and inline fragments.
	regularFields, fragmentSpreads, inlineFragments := b.separateFieldTypes(typeInfo)

	// 5. Decode regular fields from raw map.
	fieldStatements := b.fieldDecoder.DecodeFields(targetExpr, rawExpr, regularFields)
	statements = append(statements, fieldStatements...)

	// 6. Decode fragment spreads (non-pointer embedded fields with json:"-").
	fragmentStatements := b.decodeFragmentSpreads(fragmentSpreads)
	statements = append(statements, fragmentStatements...)

	// 7. Decode inline fragments (__typename based).
	inlineStatements := b.inlineDecoder.DecodeInlineFragments(targetExpr, rawExpr, inlineFragments)
	statements = append(statements, inlineStatements...)

	// 8. Return nil on success.
	statements = append(statements, &ReturnStatement{Value: "nil"})

	return statements
}

// createFragmentUnmarshalStmt は fragment spread フィールドの Unmarshal ステートメントを生成する。
//
// このメソッドは純粋関数として、副作用なく Statement を生成する。
//
// パラメータ:
//   - field: fragment spread フィールドの情報
//
// 戻り値:
//   - Statement: json.Unmarshal を呼び出すエラーチェックステートメント
func (b *UnmarshalBuilder) createFragmentUnmarshalStmt(field FieldInfo) Statement {
	fieldExpr := fmt.Sprintf("&t.%s", field.Name)
	return &ErrorCheckStatement{
		ErrorExpr: fmt.Sprintf("json.Unmarshal(data, %s)", fieldExpr),
		Body: []Statement{
			&ReturnStatement{Value: "err"},
		},
	}
}

// decodeNestedFields は fragment spread フィールドの SubFields を処理する。
//
// SubFields の分類 + 再帰処理を行い、fragment spreads と inline fragments を処理する。
//
// パラメータ:
//   - parentField: 親の fragment spread フィールド
//
// 戻り値:
//   - []Statement: SubFields をデコードするステートメントのリスト
func (b *UnmarshalBuilder) decodeNestedFields(parentField FieldInfo) []Statement {
	embeddedTargetExpr := fmt.Sprintf("t.%s", parentField.Name)
	_, subFragmentSpreads, subInlineFragments := b.separateFieldTypesAt(
		parentField.SubFields,
		embeddedTargetExpr,
	)

	var statements []Statement

	// Fragment spreads の再帰処理（明示的）
	subFragmentStatements := b.decodeFragmentSpreads(subFragmentSpreads)
	statements = append(statements, subFragmentStatements...)

	// Inline fragments の処理
	subInlineStatements := b.inlineDecoder.DecodeInlineFragments(
		embeddedTargetExpr,
		"raw",
		subInlineFragments,
	)
	statements = append(statements, subInlineStatements...)

	return statements
}

// decodeSingleFragmentSpread は単一の fragment spread フィールドを処理する。
//
// Unmarshal ステートメントの生成と、SubFields がある場合の再帰処理を行う。
//
// パラメータ:
//   - field: fragment spread フィールドの情報
//
// 戻り値:
//   - []Statement: このフィールドをデコードするステートメントのリスト
func (b *UnmarshalBuilder) decodeSingleFragmentSpread(field FieldInfo) []Statement {
	var statements []Statement

	// Unmarshal statement の生成
	unmarshalStmt := b.createFragmentUnmarshalStmt(field)
	statements = append(statements, unmarshalStmt)

	// SubFields がある場合は再帰処理
	if len(field.SubFields) > 0 {
		subStatements := b.decodeNestedFields(field)
		statements = append(statements, subStatements...)
	}

	return statements
}

// decodeFragmentSpreads は json:"-" を持つ埋め込みフィールドをアンマーシャルするステートメントを生成する。
//
// このメソッドはイミュータブルな設計に従い、新しいステートメントスライスを返す。
// 副作用を排除することで、コードの予測可能性とテストの容易性を向上させている。
//
// パラメータ:
//   - fragmentSpreads: fragment spread フィールドのリスト
//
// 戻り値:
//   - []Statement: 全ての fragment spreads をデコードするステートメントのリスト
func (b *UnmarshalBuilder) decodeFragmentSpreads(fragmentSpreads []FieldInfo) []Statement {
	var statements []Statement
	for _, field := range fragmentSpreads {
		fieldStatements := b.decodeSingleFragmentSpread(field)
		statements = append(statements, fieldStatements...)
	}
	return statements
}

// separateFieldTypes は通常フィールド、fragment spreads、inline fragments を分類する。
//
// トップレベル（targetExpr="t"）でのフィールド分類のメインエントリーポイント。
//
// パラメータ:
//   - typeInfo: 分類対象の型情報
//
// 戻り値:
//   - []FieldInfo: 通常フィールドのリスト
//   - []FieldInfo: fragment spreads のリスト
//   - []InlineFragmentInfo: inline fragments のリスト
func (b *UnmarshalBuilder) separateFieldTypes(typeInfo TypeInfo) ([]FieldInfo, []FieldInfo, []InlineFragmentInfo) {
	return b.separateFieldTypesList(typeInfo.Fields)
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
