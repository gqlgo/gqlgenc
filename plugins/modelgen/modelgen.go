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

// mutateHook は、使われている Input 型と Enum 型だけを生成するよう ModelBuild をフィルタする。
// レスポンスの形は querygen の専用型が表現するため、スキーマの Object / Interface / Union の
// モデルはクライアントから参照されない。クライアントが参照するスキーマ由来の型は Input と Enum だけ。
func mutateHook(cfg *config.Config, usedTypes map[string]bool) func(b *modelgen.ModelBuild) *modelgen.ModelBuild {
	return func(build *modelgen.ModelBuild) *modelgen.ModelBuild {
		schema := cfg.GQLGenConfig.Schema

		// Input
		inputObjects := make([]*modelgen.Object, 0, len(build.Models))
		for _, model := range build.Models {
			typeDef := schema.Types[model.Name]
			if typeDef != nil && typeDef.Kind == ast.InputObject && usedTypes[model.Name] {
				inputObjects = append(inputObjects, model)
			}
		}
		build.Models = inputObjects

		// enum
		enums := make([]*modelgen.Enum, 0, len(build.Enums))
		for _, enum := range build.Enums {
			if usedTypes[enum.Name] {
				enums = append(enums, enum)
			}
		}
		build.Enums = enums

		// Interface / Union
		// Interface と Union のモデルは build.Models ではなく build.Interfaces に入るため、
		// ここで nil にして生成対象から外す。
		build.Interfaces = nil

		return build
	}
}
