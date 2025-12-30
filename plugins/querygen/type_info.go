package querygen

import "go/types"

// TypeInfo represents analyzed type information for code generation
type TypeInfo struct {
	Named                   *types.Named
	Struct                  *types.Struct
	TypeName                string
	Fields                  []FieldInfo
	ShouldGenerateUnmarshal bool
}

// FieldInfo は構造体フィールドの情報を表す
type FieldInfo struct {
	Name             string
	Type             types.Type
	TypeName         string
	JSONTag          string
	IsExported       bool
	IsEmbedded       bool
	IsInlineFragment bool
	IsPointer        bool
	PointerElemType  string
	SubFields        []FieldInfo // 埋め込みフィールドの場合、埋め込み構造体のフィールドを含む
}

// InlineFragmentInfo は inline fragment フィールドを表す
type InlineFragmentInfo struct {
	Field       FieldInfo
	FieldExpr   string
	ElemTypeStr string
}
