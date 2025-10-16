package builder

import (
	"fmt"

	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/decoder"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// UnmarshalBuilder builds UnmarshalJSON method statements
type UnmarshalBuilder struct {
	fieldDecoder  *decoder.FieldDecoder
	inlineDecoder *decoder.InlineFragmentDecoder
}

// NewUnmarshalBuilder creates a new UnmarshalBuilder
func NewUnmarshalBuilder() *UnmarshalBuilder {
	return &UnmarshalBuilder{
		fieldDecoder:  decoder.NewFieldDecoder(),
		inlineDecoder: decoder.NewInlineFragmentDecoder(),
	}
}

// BuildUnmarshalMethod constructs the complete UnmarshalJSON method body
func (b *UnmarshalBuilder) BuildUnmarshalMethod(typeInfo model.TypeInfo) []model.Statement {
	var statements []model.Statement
	typeName := typeInfo.TypeName

	// 1. Declare raw map variable (using jsontext.Value for json/v2)
	statements = append(statements, &model.VariableDecl{
		Name: "raw",
		Type: "map[string]jsontext.Value",
	})

	// 2. Unmarshal data into raw map
	statements = append(statements, &model.ErrorCheckStatement{
		ErrorExpr: "json.Unmarshal(data, &raw)",
		Body: []model.Statement{
			&model.ReturnStatement{Value: "err"},
		},
	})

	// 3. Use Alias pattern to unmarshal all fields with default behavior
	statements = append(statements, &model.RawStatement{
		Code: fmt.Sprintf("type Alias %s", typeName),
	})
	statements = append(statements, &model.VariableDecl{
		Name: "aux",
		Type: "Alias",
	})
	statements = append(statements, &model.ErrorCheckStatement{
		ErrorExpr: "json.Unmarshal(data, &aux)",
		Body: []model.Statement{
			&model.ReturnStatement{Value: "err"},
		},
	})
	statements = append(statements, &model.RawStatement{
		Code: fmt.Sprintf("*t = %s(aux)", typeName),
	})

	// 4. Define target and raw expressions for field decoding
	targetExpr := "t"
	rawExpr := "raw"

	// 5. Separate regular fields, fragment spreads, and inline fragments
	regularFields, fragmentSpreads, inlineFragments := b.categorizeFields(typeInfo)

	// 6. Decode regular fields from raw map
	// Note: Although the Alias pattern unmarshals the data, we need to explicitly
	// unmarshal regular fields to ensure nested struct UnmarshalJSON methods are called correctly.
	// This is necessary due to json/v2 experimental behavior.
	fieldStatements := b.fieldDecoder.DecodeFields(targetExpr, rawExpr, regularFields)
	statements = append(statements, fieldStatements...)

	// 7. Decode fragment spreads (non-pointer embedded fields with json:"-")
	// Note: We only unmarshal the embedded field as a whole, not individual sub-fields.
	// This is more efficient than the previous approach which unmarshaled each sub-field individually.
	b.decodeFragmentSpreads(&statements, fragmentSpreads)

	// 8. Decode inline fragments (__typename based)
	inlineStatements := b.inlineDecoder.DecodeInlineFragments(targetExpr, rawExpr, inlineFragments)
	statements = append(statements, inlineStatements...)

	// 9. Return nil on success
	statements = append(statements, &model.ReturnStatement{Value: "nil"})

	return statements
}

// decodeFragmentSpreads generates statements to unmarshal embedded fields with json:"-"
func (b *UnmarshalBuilder) decodeFragmentSpreads(statements *[]model.Statement, fragmentSpreads []model.FieldInfo) {
	for _, field := range fragmentSpreads {
		fieldExpr := fmt.Sprintf("&t.%s", field.Name)
		embeddedTargetExpr := fmt.Sprintf("t.%s", field.Name)

		// Unmarshal the embedded field as a whole
		// Note: This automatically sets all sub-fields of the embedded field.
		// We do NOT need to individually unmarshal sub-fields, which is the key optimization.
		*statements = append(*statements, &model.ErrorCheckStatement{
			ErrorExpr: fmt.Sprintf("json.Unmarshal(data, %s)", fieldExpr),
			Body: []model.Statement{
				&model.ReturnStatement{Value: "err"},
			},
		})

		// Process only nested embedded fields and inline fragments (not regular sub-fields)
		if len(field.SubFields) > 0 {
			// Only process embedded fields and inline fragments within this embedded field
			// Regular fields are already handled by the Unmarshal above
			_, subFragmentSpreads, subInlineFragments := b.categorizeFieldsListWithPath(field.SubFields, embeddedTargetExpr)

			// Recursively decode nested embedded fields
			b.decodeFragmentSpreads(statements, subFragmentSpreads)

			// Decode inline fragments of the embedded field
			subInlineStatements := b.inlineDecoder.DecodeInlineFragments(embeddedTargetExpr, "raw", subInlineFragments)
			*statements = append(*statements, subInlineStatements...)
		}
	}
}

// categorizeFields separates regular fields, fragment spreads, and inline fragments
func (b *UnmarshalBuilder) categorizeFields(typeInfo model.TypeInfo) ([]model.FieldInfo, []model.FieldInfo, []model.InlineFragmentInfo) {
	return b.categorizeFieldsList(typeInfo.Fields)
}

// categorizeFieldsList separates a list of fields into regular fields, fragment spreads, and inline fragments
func (b *UnmarshalBuilder) categorizeFieldsList(fields []model.FieldInfo) ([]model.FieldInfo, []model.FieldInfo, []model.InlineFragmentInfo) {
	return b.categorizeFieldsListWithPath(fields, "t")
}

// categorizeFieldsListWithPath separates a list of fields with a custom parent path
func (b *UnmarshalBuilder) categorizeFieldsListWithPath(fields []model.FieldInfo, parentPath string) ([]model.FieldInfo, []model.FieldInfo, []model.InlineFragmentInfo) {
	var regularFields []model.FieldInfo
	var fragmentSpreads []model.FieldInfo
	var inlineFragments []model.InlineFragmentInfo

	for _, field := range fields {
		if field.IsInlineFragment {
			// Pointer inline fragments (__typename based)
			inlineFragments = append(inlineFragments, model.InlineFragmentInfo{
				Field:       field,
				FieldExpr:   fmt.Sprintf("%s.%s", parentPath, field.Name),
				ElemTypeStr: field.PointerElemType,
			})
		} else if field.IsEmbedded && (field.JSONTag == "" || field.JSONTag == "-") {
			// Fragment spreads (non-pointer embedded fields with json:"-")
			fragmentSpreads = append(fragmentSpreads, field)
		} else {
			// Regular fields with JSON tags
			regularFields = append(regularFields, field)
		}
	}

	return regularFields, fragmentSpreads, inlineFragments
}
