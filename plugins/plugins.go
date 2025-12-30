package plugins

import (
	"fmt"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/plugins/clientgen"
	"github.com/Yamashou/gqlgenc/v3/plugins/modelgen"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen"
)

func GenerateCode(cfg *config.Config) error {
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgen Plugin

	// modelgen
	if cfg.GQLGenConfig.Model.IsDefined() {
		modelGen := modelgen.New(cfg, cfg.GQLGencConfig.OperationQueryDocuments)
		if err := modelGen.MutateConfig(cfg.GQLGenConfig); err != nil {
			return fmt.Errorf("%s failed: %w", modelGen.Name(), err)
		}
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgenc Plugin

	// generate template sources
	operations := codegen.NewOperationGenerator(cfg).CreateOperations(cfg.GQLGencConfig.QueryDocument, cfg.GQLGencConfig.OperationQueryDocuments)
	goTypes := codegen.NewGoTypeGenerator(cfg).CreateGoTypes(cfg.GQLGencConfig.QueryDocument.Operations)

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
