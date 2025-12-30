package querygen

import (
	"fmt"
	"strings"
)

// InlineFragmentDecoder は inline fragments をデコードするステートメントを生成する。
type InlineFragmentDecoder struct{}

// NewInlineFragmentDecoder は新しい InlineFragmentDecoder を作成する。
func NewInlineFragmentDecoder() *InlineFragmentDecoder {
	return &InlineFragmentDecoder{}
}

// DecodeInlineFragments は __typename を使って inline fragments をデコードするステートメントを作成する。
//
// Inline fragments は GraphQL における型条件付きフィールドで、オブジェクトの実際の型に基づいて
// 選択される。このメソッドは以下のようなコードを生成する:
//
//	var typeName_t string
//	if typename, ok := raw["__typename"]; ok {
//	    json.Unmarshal(typename, &typeName_t)
//	}
//	switch typeName_t {
//	case "User":
//	    t.User = &UserFragment{}
//	    if err := json.Unmarshal(data, t.User); err != nil {
//	        return err
//	    }
//	case "Post":
//	    t.Post = &PostFragment{}
//	    if err := json.Unmarshal(data, t.Post); err != nil {
//	        return err
//	    }
//	}
//
// パラメータ:
//   - targetExpr: ターゲット構造体の式（例: "t"）
//   - rawExpr: raw JSON マップの式（例: "raw"）
//   - fragments: デコードする inline fragment フィールド
//
// 戻り値:
//   - []Statement: inline fragments をデコードするステートメントのリスト（空の場合は nil）
func (d *InlineFragmentDecoder) DecodeInlineFragments(targetExpr, rawExpr string, fragments []InlineFragmentInfo) []Statement {
	if len(fragments) == 0 {
		return nil
	}

	// Create unique variable name for typename
	typeNameVar := fmt.Sprintf("typeName_%s", strings.ReplaceAll(targetExpr, ".", "_"))

	var statements []Statement

	// 1. Declare typename variable
	statements = append(statements, &VariableDecl{
		Name: typeNameVar,
		Type: "string",
	})

	// 2. Extract __typename from raw
	statements = append(statements, &IfStatement{
		Condition: fmt.Sprintf(`typename, ok := %s["__typename"]; ok`, rawExpr),
		Body: []Statement{
			&RawStatement{
				Code: fmt.Sprintf("json.Unmarshal(typename, &%s)", typeNameVar),
			},
		},
	})

	// 3. Switch on typename
	switchCases := d.createSwitchCases(fragments)
	statements = append(statements, &SwitchStatement{
		Expr:  typeNameVar,
		Cases: switchCases,
	})

	return statements
}

// createSwitchCases は各 inline fragment の switch case を構築する。
//
// 各 case は:
//  1. 新しいインスタンスでポインタフィールドを初期化
//  2. 完全な JSON データをポインタにアンマーシャル
//
// case の値はフィールド名で、JSON の __typename と一致する必要がある。
//
// パラメータ:
//   - fragments: inline fragment フィールドのリスト
//
// 戻り値:
//   - []SwitchCase: switch 文の case のリスト
func (d *InlineFragmentDecoder) createSwitchCases(fragments []InlineFragmentInfo) []SwitchCase {
	cases := make([]SwitchCase, 0, len(fragments))

	for _, frag := range fragments {
		caseBody := []Statement{
			// Initialize the pointer
			&Assignment{
				Target: frag.FieldExpr,
				Value:  fmt.Sprintf("&%s{}", frag.ElemTypeStr),
			},
			// Unmarshal into it
			&ErrorCheckStatement{
				ErrorExpr: fmt.Sprintf("json.Unmarshal(data, %s)", frag.FieldExpr),
				Body: []Statement{
					&ReturnStatement{Value: "err"},
				},
			},
		}

		cases = append(cases, SwitchCase{
			Value: frag.Field.Name,
			Body:  caseBody,
		})
	}

	return cases
}
