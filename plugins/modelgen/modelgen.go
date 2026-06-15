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

		// レスポンスの形は querygen の専用型が表現するため、スキーマの Object / Interface /
		// Union のモデルはクライアントから参照されない。クライアントが参照するスキーマ由来の型は
		// Input と Enum だけなので、使われている Input 型と Enum 型だけを生成する。

		// Input
		var inputObjects []*modelgen.Object
		for _, model := range build.Models {
			typeDef := schema.Types[model.Name]
			if typeDef != nil && typeDef.Kind == ast.InputObject && usedTypes[model.Name] {
				inputObjects = append(inputObjects, model)
			}
		}
		build.Models = inputObjects

		// enum
		var enums []*modelgen.Enum
		for _, enum := range build.Enums {
			if usedTypes[enum.Name] {
				enums = append(enums, enum)
			}
		}
		build.Enums = enums

		// Interface と Union のモデルは build.Models ではなく build.Interfaces に入るため、
		// ここで nil にして生成対象から外す。
		build.Interfaces = nil

		return build
	}
}
