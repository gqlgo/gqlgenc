package querygen

import (
	"go/types"
	"reflect"
	"strings"

	"github.com/99designs/gqlgen/codegen/templates"
)

// FieldAnalyzer はGo構造体のフィールドを解析し、FieldInfo構造体のリストを構築する。
// 埋め込みフィールドの再帰的解析、インラインフラグメントの検出、
// JSONタグの解析などを行い、GraphQLクエリの型情報を抽出する。
// また、フィールドの分類（inline fragment、fragment spread、通常フィールド）も担当する。
type FieldAnalyzer struct{}

// NewFieldAnalyzer は新しい FieldAnalyzer を作成する。
func NewFieldAnalyzer() *FieldAnalyzer {
	return &FieldAnalyzer{}
}

// AnalyzeFields は構造体内の全フィールドを解析し、フィールド情報を抽出する。
//
// このメソッドは各フィールドを処理し:
//  - フィールドマッピング用の JSON タグを抽出
//  - inline fragments を検出（json:"-" を持つポインタフィールド）
//  - fragment spreads を識別（json:"-" を持つ埋め込みフィールド）
//  - 埋め込み構造体の SubFields を再帰的に解析
//
// shouldGenerateUnmarshal コールバックは、埋め込み型が独自の UnmarshalJSON を
// 生成すべきか、親にフラット化されるべきかを判定する。
//
// パラメータ:
//   - structType: 解析対象の構造体型
//   - shouldGenerateUnmarshal: UnmarshalJSON 生成判定コールバック
//
// 戻り値:
//   - []FieldInfo: 解析されたフィールド情報のリスト
func (a *FieldAnalyzer) AnalyzeFields(
	structType *types.Struct,
	shouldGenerateUnmarshal func(*types.Named) bool,
) []FieldInfo {
	fields := make([]FieldInfo, 0, structType.NumFields())

	for i := range structType.NumFields() {
		field := structType.Field(i)
		tag := structType.Tag(i)

		info := a.analyzeField(field, tag, shouldGenerateUnmarshal)
		fields = append(fields, info)
	}

	return fields
}

// analyzeField は単一フィールドを解析し、その FieldInfo を返す。
//
// 解析には以下が含まれる:
//  - フィールド名、型、JSON タグの抽出
//  - FieldClassifier による inline fragments の検出
//  - SubFields 再帰を使った埋め込みフィールド（fragment spreads）の処理
//
// 特殊ケース: 独自の UnmarshalJSON メソッドを持つ埋め込みフィールドは
// 再帰的に解析されない - それら自身がアンマーシャリングを処理する。
//
// パラメータ:
//   - field: 解析対象のフィールド変数
//   - tag: 構造体タグの文字列
//   - shouldGenerateUnmarshal: UnmarshalJSON 生成判定コールバック
//
// 戻り値:
//   - FieldInfo: 解析されたフィールド情報
func (a *FieldAnalyzer) analyzeField(
	field *types.Var,
	tag string,
	shouldGenerateUnmarshal func(*types.Named) bool,
) FieldInfo {
	info := FieldInfo{
		Name:       field.Name(),
		Type:       field.Type(),
		TypeName:   templates.CurrentImports.LookupType(field.Type()),
		JSONTag:    a.parseJSONTag(tag),
		IsExported: field.Exported(),
		IsEmbedded: field.Anonymous(),
	}

	if a.IsInlineFragment(field, tag) {
		info.IsInlineFragment = true

		if ptrType, ok := field.Type().(*types.Pointer); ok {
			info.IsPointer = true
			elemType := ptrType.Elem()
			info.PointerElemType = templates.CurrentImports.LookupType(elemType)
		}
	}

	// 埋め込みフィールドでインラインフラグメントでない場合の特別処理
	// GraphQLのフラグメントスプレッドに対応するため、埋め込みフィールドは
	// 独自のUnmarshalJSONメソッドを持つ場合と、親の型に展開される場合がある
	if info.IsEmbedded && !info.IsInlineFragment {
		if embeddedNamed := unwrapToNamedStruct(field.Type()); embeddedNamed != nil {
			// 埋め込み型が独自のUnmarshalJSONを持つ場合は、サブフィールドを解析せず早期リターン
			if shouldGenerateUnmarshal(embeddedNamed) {
				return info
			}
		}

		// 埋め込みフィールドのサブフィールドを再帰的に解析
		// これにより、ネストした埋め込み構造もフラット化される
		if embeddedStruct := unwrapToStruct(field.Type()); embeddedStruct != nil {
			info.SubFields = a.AnalyzeFields(embeddedStruct, shouldGenerateUnmarshal)
		}
	}

	return info
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
func (a *FieldAnalyzer) IsInlineFragment(field *types.Var, tag string) bool {
	if !field.Exported() {
		return false
	}

	jsonTag := a.parseJSONTag(tag)
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
//	    // その他のフィールド..
//	}
//
// パラメータ:
//   - field: チェック対象のフィールド情報
//
// 戻り値:
//   - bool: fragment spread フィールドの場合は true
func (a *FieldAnalyzer) IsFragmentSpread(field FieldInfo) bool {
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
func (a *FieldAnalyzer) IsRegularField(field FieldInfo) bool {
	return !field.IsInlineFragment && !a.IsFragmentSpread(field)
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
func (a *FieldAnalyzer) parseJSONTag(tag string) string {
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
