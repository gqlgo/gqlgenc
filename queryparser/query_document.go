package queryparser

import (
	"fmt"

	"github.com/99designs/gqlgen/codegen/templates"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

// QueryDocument parses and validate query sources.
func QueryDocument(schema *ast.Schema, querySources []*ast.Source) (*ast.QueryDocument, error) {
	var queryDocument ast.QueryDocument

	for _, querySource := range querySources {
		query, gqlerr := parser.ParseQuery(querySource)
		if gqlerr != nil {
			return nil, fmt.Errorf(": %w", gqlerr)
		}

		queryDocument.Operations = append(queryDocument.Operations, query.Operations...)
		queryDocument.Fragments = append(queryDocument.Fragments, query.Fragments...)
	}

	if errs := validator.Validate(schema, &queryDocument); errs != nil {
		return nil, fmt.Errorf(": %w", errs)
	}

	if err := isUniqueOperationName(queryDocument.Operations); err != nil {
		return nil, fmt.Errorf("is not unique operation name: %w", err)
	}

	return &queryDocument, nil
}

func isUniqueOperationName(operations ast.OperationList) error {
	operationNames := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if _, ok := operationNames[templates.ToGo(operation.Name)]; ok {
			return fmt.Errorf("duplicate operation: %s", operation.Name)
		}
	}

	return nil
}

func OperationQueryDocuments(schema *ast.Schema, operations ast.OperationList) ([]*ast.QueryDocument, error) {
	queryDocuments := make([]*ast.QueryDocument, 0, len(operations))

	for _, operation := range operations {
		fragments := fragmentsInOperationDefinition(operation)

		queryDocument := &ast.QueryDocument{
			Operations: ast.OperationList{operation},
			Fragments:  fragments,
			Position:   nil,
		}

		if errs := validator.Validate(schema, queryDocument); errs != nil {
			return nil, fmt.Errorf(": %w", errs)
		}

		queryDocuments = append(queryDocuments, queryDocument)
	}

	return queryDocuments, nil
}

func fragmentsInOperationDefinition(operation *ast.OperationDefinition) ast.FragmentDefinitionList {
	fragments := fragmentsInOperationWalker(operation.SelectionSet)
	uniqueFragments := fragmentsUnique(fragments)

	return uniqueFragments
}

func fragmentsUnique(fragments ast.FragmentDefinitionList) ast.FragmentDefinitionList {
	seenFragments := make(map[string]struct{}, len(fragments))
	uniqueFragments := make(ast.FragmentDefinitionList, 0, len(fragments))

	for _, fragment := range fragments {
		if _, ok := seenFragments[fragment.Name]; ok {
			continue
		}

		uniqueFragments = append(uniqueFragments, fragment)
		seenFragments[fragment.Name] = struct{}{}
	}

	return uniqueFragments
}

func fragmentsInOperationWalker(selectionSet ast.SelectionSet) ast.FragmentDefinitionList {
	var fragments ast.FragmentDefinitionList

	for _, selection := range selectionSet {
		var selectionSet ast.SelectionSet
		switch selection := selection.(type) {
		case *ast.Field:
			selectionSet = selection.SelectionSet
		case *ast.InlineFragment:
			selectionSet = selection.SelectionSet
		case *ast.FragmentSpread:
			fragments = append(fragments, selection.Definition)
			selectionSet = selection.Definition.SelectionSet
		}

		fragments = append(fragments, fragmentsInOperationWalker(selectionSet)...)
	}

	return fragments
}

// TypesFromQueryDocuments returns a map of type names used in query document arguments.
func TypesFromQueryDocuments(schema *ast.Schema, queryDocuments []*ast.QueryDocument) map[string]bool {
	usedTypes := make(map[string]bool)
	processedTypes := make(map[string]bool)

	for _, doc := range queryDocuments {
		for _, op := range doc.Operations {
			// Collect types from variable definitions
			for _, v := range op.VariableDefinitions {
				typeFromTypeReference(v.Type, usedTypes)
				// Recursively collect input object fields
				if typeName := v.Type.Name(); typeName != "" {
					if def, ok := schema.Types[typeName]; ok && def.IsInputType() {
						inputObjectFieldsWithCycle(def, schema, usedTypes, processedTypes)
					}
				}
			}
		}
	}

	return usedTypes
}

func inputObjectFieldsWithCycle(def *ast.Definition, schema *ast.Schema, usedTypes, processedTypes map[string]bool) {
	if processedTypes[def.Name] {
		return
	}

	processedTypes[def.Name] = true
	usedTypes[def.Name] = true

	for _, field := range def.Fields {
		var typeName string
		// Traverse to the element type of a list type
		switch {
		case field.Type == nil:
			// No type, nothing to do
			continue
		case field.Type.Elem != nil:
			// Handle slices
			typeName = field.Type.Elem.NamedType
		case field.Type.NamedType != "":
			// Handle scalar named types
			typeName = field.Type.NamedType
		}

		if typeName != "" {
			usedTypes[typeName] = true
			// Recursively collect input type fields
			if fieldDef, ok := schema.Types[typeName]; ok && fieldDef.IsInputType() {
				inputObjectFieldsWithCycle(fieldDef, schema, usedTypes, processedTypes)
			}
		}
	}
}

// typeFromTypeReference is a helper function to collect type names from type references.
func typeFromTypeReference(t *ast.Type, usedTypes map[string]bool) {
	if t == nil {
		return
	}

	if t.NamedType != "" {
		usedTypes[t.NamedType] = true
	}

	typeFromTypeReference(t.Elem, usedTypes)
}
