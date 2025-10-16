package model

import "go/types"

// TypeInfo represents analyzed type information for code generation
type TypeInfo struct {
	Named                   *types.Named
	Struct                  *types.Struct
	TypeName                string
	Fields                  []FieldInfo
	ShouldGenerateUnmarshal bool
}

// FieldInfo represents information about a struct field
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
	SubFields        []FieldInfo // For embedded fields, contains the fields of the embedded struct
}

// InlineFragmentInfo represents an inline fragment field
type InlineFragmentInfo struct {
	Field       FieldInfo
	FieldExpr   string
	ElemTypeStr string
}
