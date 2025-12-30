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

// NewFieldClassifier creates a new FieldClassifier
func NewFieldClassifier() *FieldClassifier {
	return &FieldClassifier{}
}

// IsInlineFragment checks if a field is an inline fragment field.
// Inline fragment fields are:
// - Exported
// - Have no JSON tag or json:"-"
// - Are pointer types
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

// IsFragmentSpread checks if a field is a fragment spread field.
// Fragment spread fields are embedded and have json:"-" or no JSON tag.
func (c *FieldClassifier) IsFragmentSpread(field FieldInfo) bool {
	return field.IsEmbedded && (field.JSONTag == "" || field.JSONTag == "-")
}

// IsRegularField checks if a field is a regular (non-special) field.
func (c *FieldClassifier) IsRegularField(field FieldInfo) bool {
	return !field.IsInlineFragment && !c.IsFragmentSpread(field)
}

// parseJSONTag extracts the JSON field name from a struct tag
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
