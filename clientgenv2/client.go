package clientgenv2

import (
	"fmt"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin"
	gqlgencConfig "github.com/gqlgo/gqlgenc/config"
	"github.com/gqlgo/gqlgenc/parsequery"
	"github.com/gqlgo/gqlgenc/querydocument"
	"github.com/vektah/gqlparser/v2/ast"
)

var _ plugin.ConfigMutator = &Plugin{}

type Plugin struct {
	queryFilePaths          []string
	queryDocument           *ast.QueryDocument
	operationQueryDocuments []*ast.QueryDocument
	Client                  config.PackageConfig
	GenerateConfig          *gqlgencConfig.GenerateConfig
}

func New(queryDocument *ast.QueryDocument, operationQueryDocuments []*ast.QueryDocument, client config.PackageConfig, generateConfig *gqlgencConfig.GenerateConfig) *Plugin {
	return &Plugin{
		queryDocument:           queryDocument,
		operationQueryDocuments: operationQueryDocuments,
		Client:                  client,
		GenerateConfig:          generateConfig,
	}
}

func NewWithQueryDocument(queryFilePaths []string, client config.PackageConfig, generateConfig *gqlgencConfig.GenerateConfig) *Plugin {
	return &Plugin{
		queryFilePaths: queryFilePaths,
		Client:         client,
		GenerateConfig: generateConfig,
	}
}

func (p *Plugin) Name() string {
	return "clientgen"
}

func (p *Plugin) MutateConfig(cfg *config.Config) error {
	queryDocument := p.queryDocument
	if queryDocument == nil {
		querySources, err := parsequery.LoadQuerySources(p.queryFilePaths)
		if err != nil {
			return fmt.Errorf("load query sources failed: %w", err)
		}

		queryDocument, err = parsequery.ParseQueryDocuments(cfg.Schema, querySources)
		if err != nil {
			return fmt.Errorf(": %w", err)
		}
	}

	var err error

	operationQueryDocuments := p.operationQueryDocuments
	if operationQueryDocuments == nil {
		operationQueryDocuments, err = querydocument.QueryDocumentsByOperations(cfg.Schema, queryDocument.Operations)
		if err != nil {
			return fmt.Errorf(": %w", err)
		}
	}

	// テンプレートと情報ソースを元にコード生成
	// Generate code from template and document source
	sourceGenerator := NewSourceGenerator(cfg, p.Client, p.GenerateConfig)
	source := NewSource(cfg.Schema, queryDocument, sourceGenerator, p.GenerateConfig)

	fragments, err := source.Fragments()
	if err != nil {
		return fmt.Errorf("generating fragment failed: %w", err)
	}

	operationResponses, err := source.OperationResponses()
	if err != nil {
		return fmt.Errorf("generating operation response failed: %w", err)
	}

	operations, err := source.Operations(operationQueryDocuments)
	if err != nil {
		return fmt.Errorf("generating operation failed: %w", err)
	}

	err = RenderTemplate(cfg, fragments, operations, operationResponses, source.ResponseSubTypes(), p.GenerateConfig, p.Client)
	if err != nil {
		return fmt.Errorf("template failed: %w", err)
	}

	return nil
}
