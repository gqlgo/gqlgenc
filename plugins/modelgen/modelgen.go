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
		// クエリで参照されない型は生成しない
		var newModels []*modelgen.Object

		for _, model := range build.Models {
			// スキーマから型定義を取得
			typeDef := cfg.GQLGenConfig.Schema.Types[model.Name]
			if typeDef == nil {
				// 型定義が見つからない場合はスキップ
				continue
			}

			// Input型は常に生成
			switch typeDef.Kind {
			case ast.InputObject:
				newModels = append(newModels, model)
			case ast.Enum, ast.Object:
				// Enum型とObject型は、クエリで使用されている場合のみ生成
				if usedTypes[model.Name] {
					newModels = append(newModels, model)
				}
			default:
				// その他の型は生成
				newModels = append(newModels, model)
			}
		}

		build.Models = newModels
		// Interfaceは維持（削除しない）

		return build
	}
}
