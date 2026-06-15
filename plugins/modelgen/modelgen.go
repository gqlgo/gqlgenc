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

		// クライアントが参照するのは Input と Enum だけ。レスポンスの形は querygen の
		// 専用型が表現し、Object / Interface / Union の型は @goFragment や autobind で供給される。
		// よって使われている Input 型と Enum 型だけを生成し、Object / Interface / Union は生成しない。
		var models []*modelgen.Object

		for _, model := range build.Models {
			typeDef := schema.Types[model.Name]
			if typeDef != nil && typeDef.Kind == ast.InputObject && usedTypes[model.Name] {
				models = append(models, model)
			}
		}

		build.Models = models

		var enums []*modelgen.Enum

		for _, enum := range build.Enums {
			if usedTypes[enum.Name] {
				enums = append(enums, enum)
			}
		}

		build.Enums = enums

		build.Interfaces = nil

		return build
	}
}
