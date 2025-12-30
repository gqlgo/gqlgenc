package querygen

import "fmt"

// UnmarshalBuilder builds UnmarshalJSON method statements.
type UnmarshalBuilder struct {
	fieldDecoder  *FieldDecoder
	inlineDecoder *InlineFragmentDecoder
}

// NewUnmarshalBuilder creates a new UnmarshalBuilder.
func NewUnmarshalBuilder() *UnmarshalBuilder {
	return &UnmarshalBuilder{
		fieldDecoder:  NewFieldDecoder(),
		inlineDecoder: NewInlineFragmentDecoder(),
	}
}

// BuildUnmarshalMethod constructs the complete UnmarshalJSON method body.
func (b *UnmarshalBuilder) BuildUnmarshalMethod(typeInfo TypeInfo) []Statement {
	var statements []Statement

	// 1. Declare raw map variable (using jsontext.Value for json/v2).
	statements = append(statements, &VariableDecl{
		Name: "raw",
		Type: "map[string]jsontext.Value",
	})

	// 2. Unmarshal data into raw map.
	statements = append(statements, &ErrorCheckStatement{
		ErrorExpr: "json.Unmarshal(data, &raw)",
		Body: []Statement{
			&ReturnStatement{Value: "err"},
		},
	})

	// 3. Define target and raw expressions for field decoding.
	targetExpr := "t"
	rawExpr := "raw"

	// 4. Separate regular fields, fragment spreads, and inline fragments.
	regularFields, fragmentSpreads, inlineFragments := b.categorizeFields(typeInfo)

	// 5. Decode regular fields from raw map.
	fieldStatements := b.fieldDecoder.DecodeFields(targetExpr, rawExpr, regularFields)
	statements = append(statements, fieldStatements...)

	// 6. Decode fragment spreads (non-pointer embedded fields with json:"-").
	fragmentStatements := b.decodeFragmentSpreads(fragmentSpreads)
	statements = append(statements, fragmentStatements...)

	// 7. Decode inline fragments (__typename based).
	inlineStatements := b.inlineDecoder.DecodeInlineFragments(targetExpr, rawExpr, inlineFragments)
	statements = append(statements, inlineStatements...)

	// 8. Return nil on success.
	statements = append(statements, &ReturnStatement{Value: "nil"})

	return statements
}

// decodeFragmentSpreads generates statements to unmarshal embedded fields with json:"-".
// このメソッドはイミュータブルな設計に従い、新しいステートメントスライスを返す。
// 副作用を排除することで、コードの予測可能性とテストの容易性を向上させている。
func (b *UnmarshalBuilder) decodeFragmentSpreads(fragmentSpreads []FieldInfo) []Statement {
	var statements []Statement

	for _, field := range fragmentSpreads {
		fieldExpr := fmt.Sprintf("&t.%s", field.Name)
		embeddedTargetExpr := fmt.Sprintf("t.%s", field.Name)

		// Unmarshal the embedded field as a whole to initialize nested unmarshallers.
		statements = append(statements, &ErrorCheckStatement{
			ErrorExpr: fmt.Sprintf("json.Unmarshal(data, %s)", fieldExpr),
			Body: []Statement{
				&ReturnStatement{Value: "err"},
			},
		})

		// Process nested embedded fields and inline fragments if present.
		if len(field.SubFields) > 0 {
			_, subFragmentSpreads, subInlineFragments := b.categorizeFieldsListWithPath(field.SubFields, embeddedTargetExpr)

			// Recursively get statements from sub fragment spreads
			subFragmentStatements := b.decodeFragmentSpreads(subFragmentSpreads)
			statements = append(statements, subFragmentStatements...)

			subInlineStatements := b.inlineDecoder.DecodeInlineFragments(embeddedTargetExpr, "raw", subInlineFragments)
			statements = append(statements, subInlineStatements...)
		}
	}

	return statements
}

// categorizeFields separates regular fields, fragment spreads, and inline fragments.
func (b *UnmarshalBuilder) categorizeFields(typeInfo TypeInfo) ([]FieldInfo, []FieldInfo, []InlineFragmentInfo) {
	return b.categorizeFieldsList(typeInfo.Fields)
}

// categorizeFieldsList separates a list of fields by their decoding strategy.
func (b *UnmarshalBuilder) categorizeFieldsList(fields []FieldInfo) ([]FieldInfo, []FieldInfo, []InlineFragmentInfo) {
	return b.categorizeFieldsListWithPath(fields, "t")
}

// categorizeFieldsListWithPath separates a list of fields with a custom parent path.
func (b *UnmarshalBuilder) categorizeFieldsListWithPath(fields []FieldInfo, parentPath string) ([]FieldInfo, []FieldInfo, []InlineFragmentInfo) {
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
