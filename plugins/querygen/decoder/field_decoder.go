package decoder

import (
	"fmt"

	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// FieldDecoder decodes JSON fields
type FieldDecoder struct{}

// NewFieldDecoder creates a new FieldDecoder
func NewFieldDecoder() *FieldDecoder {
	return &FieldDecoder{}
}

// DecodeField creates statements for decoding a JSON field
func (d *FieldDecoder) DecodeField(targetExpr, rawExpr string, field model.FieldInfo) model.Statement {
	fieldTarget := fmt.Sprintf("&%s.%s", targetExpr, field.Name)
	jsonName := field.JSONTag

	return &model.IfStatement{
		Condition: fmt.Sprintf(`value, ok := %s[%q]; ok`, rawExpr, jsonName),
		Body: []model.Statement{
			&model.ErrorCheckStatement{
				ErrorExpr: fmt.Sprintf("json.Unmarshal(value, %s)", fieldTarget),
				Body: []model.Statement{
					&model.ReturnStatement{Value: "err"},
				},
			},
		},
	}
}

// DecodeFields creates statements for all JSON fields
func (d *FieldDecoder) DecodeFields(targetExpr, rawExpr string, fields []model.FieldInfo) []model.Statement {
	var statements []model.Statement

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
