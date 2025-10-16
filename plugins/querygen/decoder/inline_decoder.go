package decoder

import (
	"fmt"
	"strings"

	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// InlineFragmentDecoder decodes inline fragments
type InlineFragmentDecoder struct{}

// NewInlineFragmentDecoder creates a new InlineFragmentDecoder
func NewInlineFragmentDecoder() *InlineFragmentDecoder {
	return &InlineFragmentDecoder{}
}

// DecodeInlineFragments creates statements for decoding inline fragments using __typename
func (d *InlineFragmentDecoder) DecodeInlineFragments(targetExpr, rawExpr string, fragments []model.InlineFragmentInfo) []model.Statement {
	if len(fragments) == 0 {
		return nil
	}

	// Create unique variable name for typename
	typeNameVar := fmt.Sprintf("typeName_%s", strings.ReplaceAll(targetExpr, ".", "_"))

	var statements []model.Statement

	// 1. Declare typename variable
	statements = append(statements, &model.VariableDecl{
		Name: typeNameVar,
		Type: "string",
	})

	// 2. Extract __typename from raw
	statements = append(statements, &model.IfStatement{
		Condition: fmt.Sprintf(`typename, ok := %s["__typename"]; ok`, rawExpr),
		Body: []model.Statement{
			&model.RawStatement{
				Code: fmt.Sprintf("json.Unmarshal(typename, &%s)", typeNameVar),
			},
		},
	})

	// 3. Switch on typename
	switchCases := d.buildSwitchCases(fragments)
	statements = append(statements, &model.SwitchStatement{
		Expr:  typeNameVar,
		Cases: switchCases,
	})

	return statements
}

// buildSwitchCases builds switch cases for each inline fragment
func (d *InlineFragmentDecoder) buildSwitchCases(fragments []model.InlineFragmentInfo) []model.SwitchCase {
	var cases []model.SwitchCase

	for _, frag := range fragments {
		caseBody := []model.Statement{
			// Initialize the pointer
			&model.Assignment{
				Target: frag.FieldExpr,
				Value:  fmt.Sprintf("&%s{}", frag.ElemTypeStr),
			},
			// Unmarshal into it
			&model.ErrorCheckStatement{
				ErrorExpr: fmt.Sprintf("json.Unmarshal(data, %s)", frag.FieldExpr),
				Body: []model.Statement{
					&model.ReturnStatement{Value: "err"},
				},
			},
		}

		cases = append(cases, model.SwitchCase{
			Value: frag.Field.Name,
			Body:  caseBody,
		})
	}

	return cases
}
