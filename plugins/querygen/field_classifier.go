package querygen

import (
	"go/types"
	"reflect"
	"strings"
)

// FieldClassifier はGraphQL型のフィールドを分類する責務を持つ。
// GraphQLのインラインフラグメント、フラグメントスプレッド、通常フィールドを識別し、
// 適切なコード生成を可能にする。
type FieldClassifier struct{}

// NewFieldClassifier は新しい FieldClassifier を作成する。
func NewFieldClassifier() *FieldClassifier {
	return &FieldClassifier{}
}

// IsInlineFragment はフィールドが inline fragment フィールドかどうかをチェックする。
//
// Inline fragments は "... on Type" を使って選択される GraphQL の型条件付きフィールドを表す。
// これらは JSON レスポンスの __typename フィールドに基づいてアンマーシャルされる。
//
// Inline fragment フィールドは以下の特徴を持つ:
//  - エクスポートされている（先頭が大文字）
//  - JSON タグがないか json:"-"（通常のアンマーシャリングでは無視される）
//  - ポインタ型（型条件が一致しない場合は nil になり得る）
//
// GraphQL の例:
//
//	query {
//	  node {
//	    ... on User { name }
//	    ... on Post { title }
//	  }
//	}
//
// 生成される Go 構造体:
//
//	type Node struct {
//	    User *UserFragment `json:"-"`  // inline fragment
//	    Post *PostFragment `json:"-"`  // inline fragment
//	}
//
// パラメータ:
//   - field: チェック対象のフィールド変数
//   - tag: 構造体タグの文字列
//
// 戻り値:
//   - bool: inline fragment フィールドの場合は true
func (c *FieldClassifier) IsInlineFragment(field *types.Var, tag string) bool {
	if !field.Exported() {
		return false
	}

	jsonTag := c.parseJSONTag(tag)
	if jsonTag != "" && jsonTag != "-" {
		return false
	}

	_, isPointer := field.Type().(*types.Pointer)
	return isPointer
}

// IsFragmentSpread はフィールドが fragment spread フィールドかどうかをチェックする。
//
// Fragment spreads は "...FragmentName" を使って親型に展開される GraphQL fragments を表す。
// これらは Go 構造体では埋め込みフィールドになる。
//
// Fragment spread フィールドは以下の特徴を持つ:
//  - IsEmbedded が true（構造体内の匿名フィールド）
//  - json:"-" または JSON タグなし（直接アンマーシャルされない）
//
// GraphQL の例:
//
//	fragment UserFields on User {
//	  id
//	  name
//	}
//
//	query {
//	  user {
//	    ...UserFields
//	  }
//	}
//
// 生成される Go 構造体:
//
//	type User struct {
//	    UserFields  // 埋め込みフィールド（fragment spread）
//	    // その他のフィールド...
//	}
//
// パラメータ:
//   - field: チェック対象のフィールド情報
//
// 戻り値:
//   - bool: fragment spread フィールドの場合は true
func (c *FieldClassifier) IsFragmentSpread(field FieldInfo) bool {
	return field.IsEmbedded && (field.JSONTag == "" || field.JSONTag == "-")
}

// IsRegularField はフィールドが通常の（特殊でない）フィールドかどうかをチェックする。
//
// 通常フィールドは json:"..." タグを使って JSON から通常通りアンマーシャルされる。
// これらは inline fragments でも fragment spreads でもない。
//
// パラメータ:
//   - field: チェック対象のフィールド情報
//
// 戻り値:
//   - bool: 通常フィールドの場合は true
func (c *FieldClassifier) IsRegularField(field FieldInfo) bool {
	return !field.IsInlineFragment && !c.IsFragmentSpread(field)
}

// parseJSONTag は構造体タグから JSON フィールド名を抽出する。
//
// 以下のようなタグを処理する:
//   - `json:"fieldName"` -> "fieldName"
//   - `json:"fieldName,omitempty"` -> "fieldName"
//   - `json:"-"` -> "-"
//   - `json:""` -> ""
//   - "" -> ""
//
// カンマとそれ以降のオプションは除去される。
//
// パラメータ:
//   - tag: 構造体タグの文字列（例: `json:"id,omitempty"`）
//
// 戻り値:
//   - string: 抽出された JSON フィールド名
func (c *FieldClassifier) parseJSONTag(tag string) string {
	if tag == "" {
		return ""
	}
	value := reflect.StructTag(tag).Get("json")
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}
	return value
}
