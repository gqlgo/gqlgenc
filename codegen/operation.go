package codegen

import (
	"bytes"
	gotypes "go/types"
	"slices"
	"strings"

	"github.com/Yamashou/gqlgenc/v3/internal/typebind"

	graphql "github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
)

type OperationGenerator struct {
	// models は、スキーマの型の名前ごとの、束縛先の Go 型 ("import/path.TypeName" の形)
	models map[string]string
	binder *typebind.Binder
	err    error
}

// NewOperationGenerator は OperationGenerator を作る。models はスキーマの型の束縛、binder は
// 束縛先の Go 型を解決する binder。
func NewOperationGenerator(models map[string]string, binder *typebind.Binder) *OperationGenerator {
	return &OperationGenerator{
		models: models,
		binder: binder,
		err:    nil,
	}
}

func (g *OperationGenerator) CreateOperations(queryDocument *graphql.QueryDocument, operationQueryDocuments []*graphql.QueryDocument) ([]*Operation, error) {
	queryDocumentsMap := queryDocumentMapByOperationName(operationQueryDocuments)

	operations := make([]*Operation, 0, len(queryDocument.Operations))
	for _, operation := range queryDocument.Operations {
		args := g.operationArguments(operation.VariableDefinitions)
		operations = append(operations, newOperation(operation, queryDocumentsMap[operation.Name], args))
	}

	if g.err != nil {
		return nil, g.err
	}

	return operations, nil
}

func (g *OperationGenerator) operationArguments(variableDefinitions graphql.VariableDefinitionList) []*OperationArgument {
	argumentTypes := make([]*OperationArgument, 0, len(variableDefinitions))
	for _, v := range variableDefinitions {
		argumentTypes = append(argumentTypes, &OperationArgument{
			Variable: v.Variable,
			Type:     scalarGoType(g.binder, g.models, v.Type, g.setErr),
			// 既定値を持つ変数は、型が non-null でも省略できる (省略するとサーバーが既定値を使う)。
			// 素の型で生成すると必ず値を送ることになり、既定値を使えない。
			Omittable: !v.Type.NonNull || v.DefaultValue != nil,
		})
	}

	return argumentTypes
}

// setErr は最初に起きたエラーだけを保持する (first-error-wins)。GoTypeGenerator.setErr と対称。
func (g *OperationGenerator) setErr(err error) {
	if err != nil && g.err == nil {
		g.err = err
	}
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
	// Omittable is true when the variable can be left out of the request: it is
	// nullable, or it has a default value (also when its type is non-null). Such
	// variables are generated as graphql.Omittable[T] so they can be omitted
	// (undefined), matching input fields and letting schema defaults apply.
	Omittable bool
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
