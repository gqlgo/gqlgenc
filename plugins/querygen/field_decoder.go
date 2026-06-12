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

// DecodeFieldCase はストリーミングデコード用の switch case を作成する。
//
// 以下のようなコードの case を生成する:
//
//	case "fieldName":
//	    if err := json.UnmarshalDecode(dec, &t.Field); err != nil {
//	        return err
//	    }
//
// パラメータ:
//   - targetExpr: ターゲット構造体の式（例: "t"）
//   - field: JSON タグを含むフィールド情報
//
// 戻り値:
//   - SwitchCase: フィールドをデコードする switch case
func (d *FieldDecoder) DecodeFieldCase(targetExpr string, field FieldInfo) SwitchCase {
	fieldTarget := fmt.Sprintf("&%s.%s", targetExpr, field.Name)

	return SwitchCase{
		Value: field.JSONTag,
		Body: []Statement{
			&ErrorCheckStatement{
				ErrorExpr: fmt.Sprintf("json.UnmarshalDecode(dec, %s)", fieldTarget),
				Body: []Statement{
					&ReturnStatement{Value: "err"},
				},
			},
		},
	}
}

// DecodeFieldCases は全 JSON フィールドの switch case を作成する。
//
// DecodeFields と同じ条件でフィールドをフィルタリングし、
// 残りの通常フィールドに対して DecodeFieldCase を生成する。
//
// パラメータ:
//   - targetExpr: ターゲット構造体の式（例: "t"）
//   - fields: フィールド情報のリスト
//
// 戻り値:
//   - []SwitchCase: 全ての通常フィールドをデコードする switch case のリスト
func (d *FieldDecoder) DecodeFieldCases(targetExpr string, fields []FieldInfo) []SwitchCase {
	cases := make([]SwitchCase, 0, len(fields))

	for _, field := range fields {
		if field.JSONTag == "" || field.JSONTag == "-" {
			continue
		}
		if !field.IsExported {
			continue
		}

		cases = append(cases, d.DecodeFieldCase(targetExpr, field))
	}

	return cases
}
