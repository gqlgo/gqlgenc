package codegen

import (
	"bytes"
	gotypes "go/types"
	"slices"
	"strings"

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
	queryDocumentsMap := queryDocumentMapByOperationName(operationQueryDocuments)

	operations := make([]*Operation, 0, len(queryDocument.Operations))
	for _, operation := range queryDocument.Operations {
		args := g.operationArguments(operation.VariableDefinitions)
		operations = append(operations, newOperation(operation, queryDocumentsMap[operation.Name], args))
	}

	return operations
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
	return resolveModelType(g.binder, g.cfg.GQLGenConfig.Models, typeName, nonNull)
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
	Kind                string
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
		Kind:                operationKind(operation.Operation),
		Args:                args,
		VariableDefinitions: operation.VariableDefinitions,
	}
}

// operationKind maps a GraphQL operation type to the client marker type name.
func operationKind(op graphql.Operation) string {
	switch op {
	case graphql.Mutation:
		return "Mutation"
	case graphql.Subscription:
		return "Subscription"
	default:
		return "Query"
	}
}

// formattedDocument はクエリドキュメントをミニファイした1行の文字列にする。
// WithCompacted + WithIndent("") でインデントと冗長な空白を除き、残る改行は
// トークン区切りにしか現れない（文字列リテラル内の改行はエスケープされる）ため、
// 空白へ置換しても安全にミニファイできる。
func formattedDocument(queryDocument *graphql.QueryDocument) string {
	var buf bytes.Buffer
	astFormatter := formatter.NewFormatter(&buf, formatter.WithCompacted(), formatter.WithIndent(""))
	astFormatter.FormatQueryDocument(queryDocument)

	return strings.ReplaceAll(strings.TrimSpace(buf.String()), "\n", " ")
}

// StripGoFragmentDirectives は @goFragment ディレクティブを AST から除去する。
// @goFragment はクライアント側のコード生成専用ディレクティブであり、サーバーへ送る
// クエリドキュメントに残してはならないため、型生成後・Document 生成前に呼ぶ。
func StripGoFragmentDirectives(queryDocument *graphql.QueryDocument) {
	for _, operation := range queryDocument.Operations {
		stripGoFragmentInSelectionSet(operation.SelectionSet)
	}
	for _, fragment := range queryDocument.Fragments {
		fragment.Directives = removeGoFragmentDirective(fragment.Directives)
		stripGoFragmentInSelectionSet(fragment.SelectionSet)
	}
}

func stripGoFragmentInSelectionSet(selectionSet graphql.SelectionSet) {
	for _, selection := range selectionSet {
		switch selection := selection.(type) {
		case *graphql.Field:
			selection.Directives = removeGoFragmentDirective(selection.Directives)
			stripGoFragmentInSelectionSet(selection.SelectionSet)
		case *graphql.InlineFragment:
			selection.Directives = removeGoFragmentDirective(selection.Directives)
			stripGoFragmentInSelectionSet(selection.SelectionSet)
		}
	}
}

func removeGoFragmentDirective(directives graphql.DirectiveList) graphql.DirectiveList {
	return slices.DeleteFunc(directives, func(d *graphql.Directive) bool {
		return d.Name == goFragmentDirectiveName
	})
}
