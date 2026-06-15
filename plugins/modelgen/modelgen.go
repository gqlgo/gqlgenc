package modelgen

import (
	"github.com/99designs/gqlgen/plugin/modelgen"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/queryparser"

	"github.com/vektah/gqlparser/v2/ast"
)

func New(cfg *config.Config, operationQueryDocuments []*ast.QueryDocument) *modelgen.Plugin {
	usedTypes := queryparser.TypesFromQueryDocuments(cfg.GQLGenConfig.Schema, operationQueryDocuments)

	return &modelgen.Plugin{
		MutateHook: mutateHook(cfg, usedTypes),
		FieldHook:  modelgen.DefaultFieldMutateHook,
	}
}

func mutateHook(cfg *config.Config, usedTypes map[string]bool) func(b *modelgen.ModelBuild) *modelgen.ModelBuild {
	return func(build *modelgen.ModelBuild) *modelgen.ModelBuild {
		schema := cfg.GQLGenConfig.Schema

		// クエリで参照されない Object 型は生成しない（Input 型は常に生成、Interface / Union / Scalar は維持）
		var models []*modelgen.Object

		for _, model := range build.Models {
			typeDef := schema.Types[model.Name]
			if typeDef == nil {
				continue
			}

			if typeDef.Kind == ast.Object && !usedTypes[model.Name] {
				continue
			}

			models = append(models, model)
		}

		build.Models = models

		// クエリで参照されない Enum 型は生成しない
		var enums []*modelgen.Enum

		for _, enum := range build.Enums {
			if usedTypes[enum.Name] {
				enums = append(enums, enum)
			}
		}

		build.Enums = enums

		return build
	}
}
