// Package plugins は、読み込んだスキーマとクエリからコードを生成する。
package plugins

import (
	"fmt"
	"slices"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/internal/typebind"
	"github.com/Yamashou/gqlgenc/v3/plugins/clientgen"
	"github.com/Yamashou/gqlgenc/v3/plugins/modelgen"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen"

	"github.com/vektah/gqlparser/v2/ast"
)

// GenerateCode は、設定された出力 (model・query・client) を生成する。
//
// gqlgenCfg は型を束縛した後の gqlgen の設定 (スキーマ・束縛・パッケージキャッシュを持つ)、
// binder は束縛先の Go 型を解決する binder、queryDocument はすべてのオペレーションとフラグメントを
// 持つドキュメント、operationQueryDocuments はオペレーションごとのドキュメント。
//
// queryDocument からは @goFragment を取り除く (サーバーへ送る Document に残さないため)。
func GenerateCode(
	cfg *config.Config,
	gqlgenCfg *gqlgenconfig.Config,
	binder *typebind.Binder,
	queryDocument *ast.QueryDocument,
	operationQueryDocuments []*ast.QueryDocument,
) error {
	// modelgen (gqlgen のプラグイン)
	if cfg.ModelGen.IsDefined() {
		warmPackageNames(cfg, gqlgenCfg)

		modelGen := modelgen.New(gqlgenCfg.Schema, cfg.ModelOnlyUsed, operationQueryDocuments)
		if err := modelGen.MutateConfig(gqlgenCfg); err != nil {
			return fmt.Errorf("%s failed: %w", modelGen.Name(), err)
		}
		// model_gen.go の書き込みで model パッケージの内容が変わるため、binder の
		// キャッシュから外す。model 出力先を bind.type.packages にも指定する構成
		// (生成型を同パッケージの手書きコードが参照する等) では、autobind 時にロードした
		// model 生成前の壊れた状態のキャッシュを codegen の型解決が見てしまう。
		// gqlgen 側の Packages キャッシュは modelgen 内部の ReloadAllPackages() で
		// 再ロード済みのため、次のロードは fallback 経由で生成後の状態を取得する。
		binder.Evict(cfg.ModelGen.ImportPath)
	}

	// modelgen が生成した model の束縛も含めるため、束縛の表は modelgen の後に取り出す
	models := boundGoTypes(gqlgenCfg.Models)

	// 型生成で @goFragment(type:) を読み取った後、サーバーへ送る Document に残さないよう
	// AST から @goFragment を除去してから Document を生成する。
	responseTypes, err := codegen.NewGoTypeGenerator(cfg, gqlgenCfg.Schema, models, binder).CreateResponseTypes(queryDocument.Operations)
	if err != nil {
		return fmt.Errorf("failed to create go types: %w", err)
	}

	codegen.StripGoFragmentDirectives(queryDocument)

	operations, err := codegen.NewOperationGenerator(models, binder).CreateOperations(queryDocument, operationQueryDocuments)
	if err != nil {
		return fmt.Errorf("failed to create operations: %w", err)
	}

	// querygen
	if cfg.QueryGen.IsDefined() {
		if err := querygen.Generate(cfg, operations, responseTypes); err != nil {
			return fmt.Errorf("querygen failed: %w", err)
		}
	}

	// clientgen
	if cfg.ClientGen.IsDefined() {
		if err := clientgen.Generate(cfg, operations); err != nil {
			return fmt.Errorf("clientgen failed: %w", err)
		}
	}

	return nil
}

// boundGoTypes は、gqlgen の束縛の表から、スキーマの型の名前ごとの束縛先の Go 型
// ("import/path.TypeName" の形) を取り出す。束縛先を複数持つ型 (built-in の Int は int・int64・int32) は、
// 最初の束縛先を使う。束縛先の無い型は含めない。
func boundGoTypes(models gqlgenconfig.TypeMap) map[string]string {
	goTypes := make(map[string]string, len(models))
	for typeName, entry := range models {
		if len(entry.Model) > 0 {
			goTypes[typeName] = entry.Model[0]
		}
	}

	return goTypes
}

// warmPackageNames は、model のファイルが import するパッケージ (束縛先のパッケージ) の名前解決を、
// 1 回の go list にまとめて先読みする。gqlgen の modelgen がテンプレートの描画中に個別に解決すると、
// 参照するパッケージごとに go list サブプロセスが起動する。autobind などで読み込み済みのパッケージは
// キャッシュから名前を写すだけでサブプロセスを起動しないため、生成の書き込みで Evict される前の
// この時点で行う。
//
// querygen と clientgen は、パッケージ名を型の情報から取るため、先読みは要らない。
func warmPackageNames(cfg *config.Config, gqlgenCfg *gqlgenconfig.Config) {
	gqlgenCfg.Packages.LoadAllNames(slices.Concat(cfg.TypeAutobind, gqlgenCfg.Models.ReferencedPackages())...)
}
