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
			return nil, fmt.Errorf("parse query: %w", gqlerr)
		}

		queryDocument.Operations = append(queryDocument.Operations, query.Operations...)
		queryDocument.Fragments = append(queryDocument.Fragments, query.Fragments...)
	}

	injectTypenames(&queryDocument)
	injectGoFragmentDirective(schema)

	if errs := validator.ValidateWithRules(schema, &queryDocument, nil); errs != nil {
		return nil, fmt.Errorf("validate query: %w", errs)
	}

	if err := isUniqueOperationName(queryDocument.Operations); err != nil {
		return nil, fmt.Errorf("is not unique operation name: %w", err)
	}

	return &queryDocument, nil
}

// typenameFieldName is the GraphQL meta-field used to discriminate interface and union types.
const typenameFieldName = "__typename"

// injectTypenames は、インラインフラグメント（`... on Type`）を含む選択セットに
// __typename フィールドを自動で追加する。interface / union のレスポンスを型判別して
// デコードするには __typename が必須だが、クエリで明示されていないことがあるため、
// パース後・検証前に補う。検証時に gqlparser が __typename の Definition を束縛するため、
// 利用者が手書きした場合と同一の結果になる。
func injectTypenames(queryDocument *ast.QueryDocument) {
	for _, operation := range queryDocument.Operations {
		operation.SelectionSet = injectTypenamesInSelectionSet(operation.SelectionSet)
	}
	for _, fragment := range queryDocument.Fragments {
		fragment.SelectionSet = injectTypenamesInSelectionSet(fragment.SelectionSet)
	}
}

// injectTypenamesInSelectionSet は選択セットを再帰的に走査し、インラインフラグメントを
// 含む各選択セットの先頭に __typename を追加する（既に存在する場合は追加しない）。
func injectTypenamesInSelectionSet(selectionSet ast.SelectionSet) ast.SelectionSet {
	hasInlineFragment := false
	for _, selection := range selectionSet {
		switch selection := selection.(type) {
		case *ast.Field:
			selection.SelectionSet = injectTypenamesInSelectionSet(selection.SelectionSet)
		case *ast.InlineFragment:
			hasInlineFragment = true
			selection.SelectionSet = injectTypenamesInSelectionSet(selection.SelectionSet)
		}
	}

	if !hasInlineFragment || hasTypenameField(selectionSet) {
		return selectionSet
	}

	typenameField := &ast.Field{
		Alias: typenameFieldName,
		Name:  typenameFieldName,
	}
	injected := make(ast.SelectionSet, 0, len(selectionSet)+1)
	injected = append(injected, typenameField)
	injected = append(injected, selectionSet...)

	return injected
}

// hasTypenameField は選択セットの直下にエイリアスなしの __typename フィールドが
// 既にあるかを返す。エイリアスされた __typename（例: kind: __typename）は、
// 生成型のフィールド名が Typename にならないため「無い」とみなし、注入対象とする。
func hasTypenameField(selectionSet ast.SelectionSet) bool {
	for _, selection := range selectionSet {
		if field, ok := selection.(*ast.Field); ok && field.Name == typenameFieldName && field.Alias == typenameFieldName {
			return true
		}
	}
	return false
}

// goFragmentDirectiveName is the client-only directive used to bind a query
// fragment or field selection to an existing Go type.
const goFragmentDirectiveName = "goFragment"

// injectGoFragmentDirective declares the client-only @goFragment directive so a
// query can bind a fragment definition or field selection to an existing Go
// type. It is stripped before the document is sent to the server.
func injectGoFragmentDirective(schema *ast.Schema) {
	if schema.Directives[goFragmentDirectiveName] != nil {
		return
	}
	schema.Directives[goFragmentDirectiveName] = &ast.DirectiveDefinition{
		Name: goFragmentDirectiveName,
		Arguments: ast.ArgumentDefinitionList{
			{Name: "type", Type: ast.NamedType("String", nil)},
		},
		Locations: []ast.DirectiveLocation{
			ast.LocationFragmentDefinition,
			ast.LocationField,
		},
	}
}

func isUniqueOperationName(operations ast.OperationList) error {
	operationNames := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		goName := templates.ToGo(operation.Name)
		if _, ok := operationNames[goName]; ok {
			return fmt.Errorf("duplicate operation: %s", operation.Name)
		}
		operationNames[goName] = struct{}{}
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

		if errs := validator.ValidateWithRules(schema, queryDocument, nil); errs != nil {
			return nil, fmt.Errorf("validate operation %q: %w", operation.Name, errs)
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

// TypesFromQueryDocuments returns a map of type names used in query documents,
// both from variable definitions (input types) and enum types from the selection set (return types).
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
			enumsFromSelectionSet(op.SelectionSet, schema, usedTypes)
		}
	}

	return usedTypes
}

func enumsFromSelectionSet(selectionSet ast.SelectionSet, schema *ast.Schema, usedTypes map[string]bool) {
	for _, selection := range selectionSet {
		switch selection := selection.(type) {
		case *ast.Field:
			if selection.Definition != nil {
				typeName := selection.Definition.Type.Name()
				if def, ok := schema.Types[typeName]; ok && def.Kind == ast.Enum {
					usedTypes[typeName] = true
				}
			}
			enumsFromSelectionSet(selection.SelectionSet, schema, usedTypes)
		case *ast.InlineFragment:
			enumsFromSelectionSet(selection.SelectionSet, schema, usedTypes)
		case *ast.FragmentSpread:
			if selection.Definition != nil {
				enumsFromSelectionSet(selection.Definition.SelectionSet, schema, usedTypes)
			}
		}
	}
}

func inputObjectFieldsWithCycle(def *ast.Definition, schema *ast.Schema, usedTypes, processedTypes map[string]bool) {
	if processedTypes[def.Name] {
		return
	}

	processedTypes[def.Name] = true
	usedTypes[def.Name] = true

	for _, field := range def.Fields {
		if field.Type == nil {
			continue
		}

		// Type.Name() は入れ子リスト ([[T!]!] など) を辿って最内の名前付き型を返す
		typeName := field.Type.Name()
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
