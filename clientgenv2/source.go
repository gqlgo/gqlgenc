package clientgenv2

import (
	"bytes"
	"fmt"
	"go/types"

	"github.com/99designs/gqlgen/codegen/templates"

	"github.com/gqlgo/gqlgenc/config"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
)

type Source struct {
	schema          *ast.Schema
	queryDocument   *ast.QueryDocument
	sourceGenerator *SourceGenerator
	generateConfig  *config.GenerateConfig
}

func NewSource(schema *ast.Schema, queryDocument *ast.QueryDocument, sourceGenerator *SourceGenerator, generateConfig *config.GenerateConfig) *Source {
	return &Source{
		schema:          schema,
		queryDocument:   queryDocument,
		sourceGenerator: sourceGenerator,
		generateConfig:  generateConfig,
	}
}

// SpreadFragmentInfo holds metadata about a fragment spread that was merged
// into a parent fragment. Used to generate conversion getters.
type SpreadFragmentInfo struct {
	// Name is the spread fragment's type name (e.g., "NameFrag").
	Name string
	// Type is the spread fragment's resolved Go type.
	Type types.Type
}

// Fragment represents a named GraphQL fragment definition and its generated Go type.
type Fragment struct {
	// Name is the fragment's name as declared in GraphQL.
	Name string
	// Type is the Go struct type generated for this fragment.
	Type types.Type
	// SpreadFragments lists fragment spreads that were flattened into this fragment.
	// Conversion getters are generated for each entry.
	SpreadFragments []*SpreadFragmentInfo
}

func (s *Source) Fragments() ([]*Fragment, error) {
	fragments := make([]*Fragment, 0, len(s.queryDocument.Fragments))
	for _, fragment := range s.queryDocument.Fragments {
		responseFields := s.sourceGenerator.NewResponseFields(fragment.SelectionSet, fragment.Name)
		if s.sourceGenerator.cfg.Models.Exists(fragment.Name) {
			return nil, fmt.Errorf("%s is duplicated", fragment.Name)
		}

		// When fragment spreads are present, apply the same merge strategy used by Operations.
		// Flatten spread fields and deduplicate them so the graphqljson decoder can
		// unmarshal all fields directly without traversing named pointer fields.
		var structType *types.Struct
		var spreadFragments []*SpreadFragmentInfo
		if s.sourceGenerator.hasFragmentSpread(responseFields) {
			// Collect spread fragment info before flattening (for conversion getter generation).
			spreadFragments = collectSpreadFragmentInfo(responseFields)

			flattened := flattenFragmentSpreads(responseFields)
			generator := NewStructGenerator(flattened)
			s.sourceGenerator.StructSources = generator.MergedStructSources(s.sourceGenerator.StructSources)
			structType = generator.GetCurrentResponseFieldList().StructType()
		} else {
			structType = responseFields.StructType()
		}

		fragment := &Fragment{
			Name:            fragment.Name,
			Type:            structType,
			SpreadFragments: spreadFragments,
		}

		fragments = append(fragments, fragment)
	}

	for _, fragment := range fragments {
		name := fragment.Name
		s.sourceGenerator.cfg.Models.Add(
			name,
			fmt.Sprintf("%s.%s", s.sourceGenerator.client.Pkg(), templates.ToGo(name)),
		)
	}

	return fragments, nil
}

type Operation struct {
	Name                string
	ResponseStructName  string
	Operation           string
	Args                []*Argument
	VariableDefinitions ast.VariableDefinitionList
}

func NewOperation(operation *ast.OperationDefinition, queryDocument *ast.QueryDocument, args []*Argument, generateConfig *config.GenerateConfig) *Operation {
	return &Operation{
		Name:                operation.Name,
		ResponseStructName:  getResponseStructName(operation, generateConfig),
		Operation:           queryString(queryDocument),
		Args:                args,
		VariableDefinitions: operation.VariableDefinitions,
	}
}

func ValidateOperationList(os ast.OperationList) error {
	err := IsUniqueName(os)
	if err != nil {
		return fmt.Errorf("is not unique operation name: %w", err)
	}

	return nil
}

func IsUniqueName(os ast.OperationList) error {
	operationNames := make(map[string]struct{})
	for _, operation := range os {
		_, exist := operationNames[templates.ToGo(operation.Name)]
		if exist {
			return fmt.Errorf("duplicate operation: %s", operation.Name)
		}
	}

	return nil
}

func (s *Source) Operations(queryDocuments []*ast.QueryDocument) ([]*Operation, error) {
	operations := make([]*Operation, 0, len(s.queryDocument.Operations))

	queryDocumentsMap := queryDocumentMapByOperationName(queryDocuments)
	operationArgsMap := s.operationArgsMapByOperationName()

	err := ValidateOperationList(s.queryDocument.Operations)
	if err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	for _, operation := range s.queryDocument.Operations {
		queryDocument := queryDocumentsMap[operation.Name]

		args := operationArgsMap[operation.Name]
		operations = append(operations, NewOperation(
			operation,
			queryDocument,
			args,
			s.generateConfig,
		))
	}

	return operations, nil
}

func queryDocumentMapByOperationName(queryDocuments []*ast.QueryDocument) map[string]*ast.QueryDocument {
	queryDocumentMap := make(map[string]*ast.QueryDocument)

	for _, queryDocument := range queryDocuments {
		operation := queryDocument.Operations[0]
		queryDocumentMap[operation.Name] = queryDocument
	}

	return queryDocumentMap
}

func queryString(queryDocument *ast.QueryDocument) string {
	var buf bytes.Buffer

	astFormatter := formatter.NewFormatter(&buf)
	astFormatter.FormatQueryDocument(queryDocument)

	return buf.String()
}

type OperationResponse struct {
	Name string
	Type types.Type
}

func (s *Source) OperationResponses() ([]*OperationResponse, error) {
	operationResponse := make([]*OperationResponse, 0, len(s.queryDocument.Operations))
	for _, operation := range s.queryDocument.Operations {
		responseFields := s.sourceGenerator.NewResponseFields(operation.SelectionSet, operation.Name)

		name := getResponseStructName(operation, s.generateConfig)
		if s.sourceGenerator.cfg.Models.Exists(name) {
			return nil, fmt.Errorf("%s is duplicated", name)
		}

		operationResponse = append(operationResponse, &OperationResponse{
			Name: name,
			Type: responseFields.StructType(),
		})
	}

	for _, operationResponse := range operationResponse {
		name := operationResponse.Name
		s.sourceGenerator.cfg.Models.Add(
			name,
			fmt.Sprintf("%s.%s", s.sourceGenerator.client.Pkg(), templates.ToGo(name)),
		)
	}

	return operationResponse, nil
}

func (s *Source) ResponseSubTypes() []*StructSource {
	return s.sourceGenerator.StructSources
}

func (s *Source) operationArgsMapByOperationName() map[string][]*Argument {
	operationArgsMap := make(map[string][]*Argument)
	for _, operation := range s.queryDocument.Operations {
		operationArgsMap[operation.Name] = s.sourceGenerator.OperationArguments(operation.VariableDefinitions)
	}

	return operationArgsMap
}

func getResponseStructName(operation *ast.OperationDefinition, generateConfig *config.GenerateConfig) string {
	name := operation.Name

	if generateConfig != nil {
		if generateConfig.Prefix != nil {
			if operation.Operation == ast.Mutation {
				name = fmt.Sprintf("%s%s", generateConfig.Prefix.Mutation, name)
			}

			if operation.Operation == ast.Query {
				name = fmt.Sprintf("%s%s", generateConfig.Prefix.Query, name)
			}
		}

		if generateConfig.Suffix != nil {
			if operation.Operation == ast.Mutation {
				name = fmt.Sprintf("%s%s", name, generateConfig.Suffix.Mutation)
			}

			if operation.Operation == ast.Query {
				name = fmt.Sprintf("%s%s", name, generateConfig.Suffix.Query)
			}
		}
	}

	return name
}
