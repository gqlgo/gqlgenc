package plugins

import (
	"fmt"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/plugins/clientgen"
	"github.com/Yamashou/gqlgenc/v3/plugins/modelgen"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen"
	"github.com/Yamashou/gqlgenc/v3/queryparser"
)

func GenerateCode(cfg *config.Config) error {
	// Load Query
	querySources, err := queryparser.LoadQuerySources(cfg.GQLGencConfig.Query)
	if err != nil {
		return fmt.Errorf("load query sources failed: %w", err)
	}

	queryDocument, err := queryparser.QueryDocument(cfg.GQLGenConfig.Schema, querySources)
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	operationQueryDocuments, err := queryparser.OperationQueryDocuments(cfg.GQLGenConfig.Schema, queryDocument.Operations)
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	// modelgen
	if cfg.GQLGenConfig.Model.IsDefined() {
		modelGen := modelgen.New(cfg, operationQueryDocuments)
		if err := modelGen.MutateConfig(cfg.GQLGenConfig); err != nil {
			return fmt.Errorf("%s failed: %w", modelGen.Name(), err)
		}
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgenc Plugins

	// generate template sources
	operations := codegen.NewOperationGenerator(cfg).CreateOperations(queryDocument, operationQueryDocuments)
	goTypes := codegen.NewGoTypeGenerator(cfg).CreateGoTypes(queryDocument.Operations)

	// querygen
	if cfg.GQLGencConfig.QueryGen.IsDefined() {
		queryGen := querygen.New(cfg, operations, goTypes)
		if err := queryGen.MutateConfig(cfg.GQLGenConfig); err != nil {
			return fmt.Errorf("%s failed: %w", queryGen.Name(), err)
		}
	}

	// clientgen
	if cfg.GQLGencConfig.ClientGen.IsDefined() {
		clientGen := clientgen.New(cfg, operations)
		if err := clientGen.MutateConfig(cfg.GQLGenConfig); err != nil {
			return fmt.Errorf("%s failed: %w", clientGen.Name(), err)
		}
	}

	return nil
}
