package codegen

import (
	"bytes"
	"fmt"
	gotypes "go/types"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/config"

	graphql "github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
)

type OperationGenerator struct {
	cfg    *config.Config
	binder *gqlgenconfig.Binder
}

func NewOperationGenerator(cfg *config.Config) *OperationGenerator {
	return &OperationGenerator{
		cfg:    cfg,
		binder: cfg.GQLGenConfig.NewBinder(),
	}
}

func (g *OperationGenerator) CreateOperations(queryDocument *graphql.QueryDocument, operationQueryDocuments []*graphql.QueryDocument) []*Operation {
	operationArgsMap := g.operationArgsMapByOperationName(queryDocument)
	queryDocumentsMap := queryDocumentMapByOperationName(operationQueryDocuments)

	operations := make([]*Operation, 0, len(queryDocument.Operations))
	for _, operation := range queryDocument.Operations {
		operationQueryDocument := queryDocumentsMap[operation.Name]
		args := operationArgsMap[operation.Name]
		operations = append(operations, newOperation(operation, operationQueryDocument, args))
	}

	return operations
}

func (g *OperationGenerator) operationArgsMapByOperationName(queryDocument *graphql.QueryDocument) map[string][]*OperationArgument {
	operationArgsMap := make(map[string][]*OperationArgument)
	for _, operation := range queryDocument.Operations {
		operationArgsMap[operation.Name] = g.operationArguments(operation.VariableDefinitions)
	}

	return operationArgsMap
}

func (g *OperationGenerator) operationArguments(variableDefinitions graphql.VariableDefinitionList) []*OperationArgument {
	argumentTypes := make([]*OperationArgument, 0, len(variableDefinitions))
	for _, v := range variableDefinitions {
		argumentTypes = append(argumentTypes, &OperationArgument{
			Variable: v.Variable,
			Type:     g.findGoTypeName(v.Type.Name(), v.Type.NonNull),
		})
	}

	return argumentTypes
}

func (g *OperationGenerator) findGoTypeName(typeName string, nonNull bool) gotypes.Type {
	goType, err := g.binder.FindTypeFromName(g.cfg.GQLGenConfig.Models[typeName].Model[0])
	if err != nil {
		// If we pass the correct typeName as per implementation, it should always be found, so we panic if not
		panic(fmt.Sprintf("%+v", err))
	}
	if !nonNull {
		goType = gotypes.NewPointer(goType)
	}

	return goType
}

func queryDocumentMapByOperationName(queryDocuments []*graphql.QueryDocument) map[string]*graphql.QueryDocument {
	queryDocumentMap := make(map[string]*graphql.QueryDocument)
	for _, queryDocument := range queryDocuments {
		operation := queryDocument.Operations[0]
		queryDocumentMap[operation.Name] = queryDocument
	}

	return queryDocumentMap
}

type Operation struct {
	Name                string
	Document            string
	Args                []*OperationArgument
	VariableDefinitions graphql.VariableDefinitionList
}

type OperationArgument struct {
	Type     gotypes.Type
	Variable string
}

func newOperation(operation *graphql.OperationDefinition, queryDocument *graphql.QueryDocument, args []*OperationArgument) *Operation {
	return &Operation{
		Name:                operation.Name,
		Document:            formattedDocument(queryDocument),
		Args:                args,
		VariableDefinitions: operation.VariableDefinitions,
	}
}

func formattedDocument(queryDocument *graphql.QueryDocument) string {
	var buf bytes.Buffer
	astFormatter := formatter.NewFormatter(&buf)
	astFormatter.FormatQueryDocument(queryDocument)

	return buf.String()
}
