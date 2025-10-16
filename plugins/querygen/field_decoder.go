package querygen

import (
	"fmt"
)

// FieldDecoder decodes JSON fields
type FieldDecoder struct{}

// NewFieldDecoder creates a new FieldDecoder
func NewFieldDecoder() *FieldDecoder {
	return &FieldDecoder{}
}

// DecodeField creates statements for decoding a JSON field
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

// DecodeFields creates statements for all JSON fields
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
