package querygen

import (
	"fmt"
)

// FieldDecoder は JSON フィールドをデコードするステートメントを生成する。
type FieldDecoder struct{}

// NewFieldDecoder は新しい FieldDecoder を作成する。
func NewFieldDecoder() *FieldDecoder {
	return &FieldDecoder{}
}

// DecodeField は JSON フィールドをデコードするステートメントを作成する。
//
// 以下のようなコードを生成する:
//
//	if value, ok := raw["fieldName"]; ok {
//	    if err := json.Unmarshal(value, &t.Field); err != nil {
//	        return err
//	    }
//	}
//
// パラメータ:
//   - targetExpr: ターゲット構造体の式（例: "t"）
//   - rawExpr: raw JSON マップの式（例: "raw"）
//   - field: JSON タグを含むフィールド情報
//
// 戻り値:
//   - Statement: フィールドをデコードする if ステートメント
func (d *FieldDecoder) DecodeField(targetExpr, rawExpr string, field FieldInfo) Statement {
	fieldTarget := fmt.Sprintf("&%s.%s", targetExpr, field.Name)
	jsonName := field.JSONTag

	return &IfStatement{
		Condition: fmt.Sprintf(`value, ok := %s[%q]; ok`, rawExpr, jsonName),
		Body: []Statement{
			&ErrorCheckStatement{
				ErrorExpr: fmt.Sprintf("json.Unmarshal(value, %s)", fieldTarget),
				Body: []Statement{
					&ReturnStatement{Value: "err"},
				},
			},
		},
	}
}

// DecodeFields は全 JSON フィールドのステートメントを作成する。
//
// このメソッドは以下をフィルタリングする:
//   - json:"-" を持つフィールド（fragment spreads と inline fragments）
//   - JSON タグがないフィールド（埋め込みフィールド）
//   - エクスポートされていないフィールド
//
// そして残りの通常フィールドに対して DecodeField ステートメントを生成する。
//
// パラメータ:
//   - targetExpr: ターゲット構造体の式（例: "t"）
//   - rawExpr: raw JSON マップの式（例: "raw"）
//   - fields: フィールド情報のリスト
//
// 戻り値:
//   - []Statement: 全ての通常フィールドをデコードするステートメントのリスト
func (d *FieldDecoder) DecodeFields(targetExpr, rawExpr string, fields []FieldInfo) []Statement {
	statements := make([]Statement, 0, len(fields))

	for _, field := range fields {
		if field.JSONTag == "" || field.JSONTag == "-" {
			continue
		}
		if !field.IsExported {
			continue
		}

		statements = append(statements, d.DecodeField(targetExpr, rawExpr, field))
	}

	return statements
}
