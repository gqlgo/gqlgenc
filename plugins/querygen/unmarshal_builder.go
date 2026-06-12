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

// BuildUnmarshalMethod は完全な UnmarshalJSONFrom メソッド本体を構築する。
//
// 生成されるメソッドは jsontext.Decoder から直接デコードする (json/v2 の
// UnmarshalerFrom)。フィールド構成によって2つのモードを使い分ける:
//
//  1. ストリーミングモード: 通常フィールドのみの型。トークンを1パスで読み、
//     各フィールドへ直接デコードする。中間バッファを作らない。
//  2. バッファモード: fragment spreads (json:"-" の埋め込みフィールド) や
//     inline fragments (__typename による型分岐) を含む型。同じ JSON データを
//     複数のターゲットへデコードする必要があるため、値全体を一度だけバッファする。
//
// パラメータ:
//   - fields: 型のフィールド情報のリスト
//
// 戻り値:
//   - []Statement: UnmarshalJSONFrom メソッド本体のステートメントリスト
func (b *UnmarshalBuilder) BuildUnmarshalMethod(fields []FieldInfo) []Statement {
	regularFields, fragmentSpreads, inlineFragments := b.separateFieldTypesList(fields)

	if len(fragmentSpreads) == 0 && len(inlineFragments) == 0 {
		return b.buildStreamingMethod(regularFields)
	}

	return b.buildBufferedMethod(regularFields, fragmentSpreads, inlineFragments)
}

// buildStreamingMethod は通常フィールドのみの型のメソッド本体を構築する。
//
// トークンを1パスで読み、メンバー名の switch で各フィールドへ
// json.UnmarshalDecode する。未知のメンバーは SkipValue で読み飛ばす。
// JSON null は値を消費してゼロ値のまま返す。
//
// パラメータ:
//   - regularFields: 通常フィールドのリスト
//
// 戻り値:
//   - []Statement: メソッド本体のステートメントリスト
func (b *UnmarshalBuilder) buildStreamingMethod(regularFields []FieldInfo) []Statement {
	var statements []Statement

	// 1. JSON null は値を消費してゼロ値のまま返す。
	statements = append(statements, &IfStatement{
		Condition: "dec.PeekKind() == 'n'",
		Body: []Statement{
			&RawStatement{Code: "_, err := dec.ReadToken()"},
			&ReturnStatement{Value: "err"},
		},
	})

	// 2. オブジェクト開始トークンを読む。
	statements = append(statements,
		&RawStatement{Code: "tok, err := dec.ReadToken()"},
		&IfStatement{
			Condition: "err != nil",
			Body:      []Statement{&ReturnStatement{Value: "err"}},
		},
		&IfStatement{
			Condition: "tok.Kind() != '{'",
			Body: []Statement{
				&ReturnStatement{Value: `fmt.Errorf("unexpected JSON kind %v, expected object", tok.Kind())`},
			},
		},
	)

	// 3. メンバー名ごとに対応するフィールドへストリーミングデコードする。
	statements = append(statements, &ForStatement{
		Condition: "dec.PeekKind() != '}'",
		Body: []Statement{
			&RawStatement{Code: "name, err := dec.ReadToken()"},
			&IfStatement{
				Condition: "err != nil",
				Body:      []Statement{&ReturnStatement{Value: "err"}},
			},
			&SwitchStatement{
				Expr:  "name.String()",
				Cases: b.fieldDecoder.DecodeFieldCases("t", regularFields),
				Default: []Statement{
					&ErrorCheckStatement{
						ErrorExpr: "dec.SkipValue()",
						Body:      []Statement{&ReturnStatement{Value: "err"}},
					},
				},
			},
		},
	})

	// 4. オブジェクト終了トークンを読む。
	statements = append(statements, &RawStatement{Code: "if _, err := dec.ReadToken(); err != nil {\n\t\treturn err\n\t}"})
	statements = append(statements, &ReturnStatement{Value: "nil"})

	return statements
}

// buildBufferedMethod は fragment を含む型のメソッド本体を構築する。
//
// 同じ JSON データを親フィールドと fragment の両方へデコードする必要が
// あるため、dec.ReadValue で値全体を一度だけバッファし、従来どおり
// raw マップ経由でフィールドをデコードする。
//
// パラメータ:
//   - regularFields: 通常フィールドのリスト
//   - fragmentSpreads: fragment spread フィールドのリスト
//   - inlineFragments: inline fragment フィールドのリスト
//
// 戻り値:
//   - []Statement: メソッド本体のステートメントリスト
func (b *UnmarshalBuilder) buildBufferedMethod(regularFields, fragmentSpreads []FieldInfo, inlineFragments []InlineFragmentInfo) []Statement {
	var statements []Statement

	// 1. 値全体を一度だけバッファする。
	statements = append(statements,
		&RawStatement{Code: "data, err := dec.ReadValue()"},
		&IfStatement{
			Condition: "err != nil",
			Body:      []Statement{&ReturnStatement{Value: "err"}},
		},
	)

	// 2. raw マップへデコードする。
	statements = append(statements, &VariableDecl{
		Name: "raw",
		Type: "map[string]jsontext.Value",
	})
	statements = append(statements, &ErrorCheckStatement{
		ErrorExpr: "json.Unmarshal(data, &raw)",
		Body: []Statement{
			&ReturnStatement{Value: "err"},
		},
	})

	// 3. Decode regular fields from raw map.
	statements = append(statements, b.fieldDecoder.DecodeFields("t", "raw", regularFields)...)

	// 4. Decode fragment spreads (non-pointer embedded fields with json:"-").
	statements = append(statements, b.decodeFragmentSpreads(fragmentSpreads)...)

	// 5. Decode inline fragments (__typename based).
	statements = append(statements, b.inlineDecoder.DecodeInlineFragments("t", "raw", inlineFragments)...)

	// 6. Return nil on success.
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
