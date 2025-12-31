package parsequery

import (
	"fmt"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

func ParseQueryDocuments(schema *ast.Schema, querySources []*ast.Source) (*ast.QueryDocument, error) {
	var queryDocument ast.QueryDocument

	for _, querySource := range querySources {
		query, gqlerr := parser.ParseQuery(querySource)
		if gqlerr != nil {
			return nil, fmt.Errorf(": %w", gqlerr)
		}

		mergeQueryDocument(&queryDocument, query)
	}

	errs := validator.Validate(schema, &queryDocument)
	if errs != nil {
		return nil, fmt.Errorf(": %w", errs)
	}

	return &queryDocument, nil
}

func mergeQueryDocument(q, other *ast.QueryDocument) {
	q.Operations = append(q.Operations, other.Operations...)
	q.Fragments = append(q.Fragments, other.Fragments...)
}
