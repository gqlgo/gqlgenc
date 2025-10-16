package querygen

import (
	"fmt"
	"strings"
)

// InlineFragmentDecoder decodes inline fragments
type InlineFragmentDecoder struct{}

// NewInlineFragmentDecoder creates a new InlineFragmentDecoder
func NewInlineFragmentDecoder() *InlineFragmentDecoder {
	return &InlineFragmentDecoder{}
}

// DecodeInlineFragments creates statements for decoding inline fragments using __typename
func (d *InlineFragmentDecoder) DecodeInlineFragments(targetExpr, rawExpr string, fragments []InlineFragmentInfo) []Statement {
	if len(fragments) == 0 {
		return nil
	}

	// Create unique variable name for typename
	typeNameVar := fmt.Sprintf("typeName_%s", strings.ReplaceAll(targetExpr, ".", "_"))

	var statements []Statement

	// 1. Declare typename variable
	statements = append(statements, &VariableDecl{
		Name: typeNameVar,
		Type: "string",
	})

	// 2. Extract __typename from raw
	statements = append(statements, &IfStatement{
		Condition: fmt.Sprintf(`typename, ok := %s["__typename"]; ok`, rawExpr),
		Body: []Statement{
			&RawStatement{
				Code: fmt.Sprintf("json.Unmarshal(typename, &%s)", typeNameVar),
			},
		},
	})

	// 3. Switch on typename
	switchCases := d.buildSwitchCases(fragments)
	statements = append(statements, &SwitchStatement{
		Expr:  typeNameVar,
		Cases: switchCases,
	})

	return statements
}

// buildSwitchCases builds switch cases for each inline fragment
func (d *InlineFragmentDecoder) buildSwitchCases(fragments []InlineFragmentInfo) []SwitchCase {
	cases := make([]SwitchCase, 0, len(fragments))

	for _, frag := range fragments {
		caseBody := []Statement{
			// Initialize the pointer
			&Assignment{
				Target: frag.FieldExpr,
				Value:  fmt.Sprintf("&%s{}", frag.ElemTypeStr),
			},
			// Unmarshal into it
			&ErrorCheckStatement{
				ErrorExpr: fmt.Sprintf("json.Unmarshal(data, %s)", frag.FieldExpr),
				Body: []Statement{
					&ReturnStatement{Value: "err"},
				},
			},
		}

		cases = append(cases, SwitchCase{
			Value: frag.Field.Name,
			Body:  caseBody,
		})
	}

	return cases
}
