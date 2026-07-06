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
	warmPackageNames(cfg)

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgen Plugin

	// modelgen
	if cfg.GQLGenConfig.Model.IsDefined() {
		modelGen := modelgen.New(cfg, cfg.GQLGencConfig.OperationQueryDocuments)
		if err := modelGen.MutateConfig(cfg.GQLGenConfig); err != nil {
			return fmt.Errorf("%s failed: %w", modelGen.Name(), err)
		}
		// model_gen.go の書き込みで model パッケージの内容が変わるため、TypeBinder の
		// キャッシュから外す。model 出力先を bind.type.packages にも指定する構成
		// (生成型を同パッケージの手書きコードが参照する等) では、autobind 時にロードした
		// model 生成前の壊れた状態のキャッシュを codegen の型解決が見てしまう。
		// gqlgen 側の Packages キャッシュは modelgen 内部の ReloadAllPackages() で
		// 再ロード済みのため、次のロードは fallback 経由で生成後の状態を取得する。
		cfg.GQLGencConfig.TypeBinder.Evict(cfg.GQLGenConfig.Model.ImportPath())
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgenc Plugin

	// generate template sources
	// 型生成で @goFragment(type:) を読み取った後、サーバーへ送る Document に残さないよう
	// AST から @goFragment を除去してから Document を生成する。
	goTypes, err := codegen.NewGoTypeGenerator(cfg).CreateGoTypes(cfg.GQLGencConfig.QueryDocument.Operations)
	if err != nil {
		return fmt.Errorf("failed to create go types: %w", err)
	}

	codegen.StripGoFragmentDirectives(cfg.GQLGencConfig.QueryDocument)

	operations, err := codegen.NewOperationGenerator(cfg).CreateOperations(cfg.GQLGencConfig.QueryDocument, cfg.GQLGencConfig.OperationQueryDocuments)
	if err != nil {
		return fmt.Errorf("failed to create operations: %w", err)
	}

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

// warmPackageNames は生成ファイルが import するパッケージの名前解決を1回の
// go list にまとめて先読みする。テンプレート描画中に templates が個別に解決すると
// 参照パッケージごとに go list サブプロセスが起動する。autobind などでロード済みの
// パッケージはキャッシュから名前を写すだけでサブプロセスを起動しないため、生成の
// 書き込みで Evict される前のこの時点で行う。
func warmPackageNames(cfg *config.Config) {
	// querygen / clientgen のテンプレートが常に import する静的パッケージ
	staticPaths := []string{
		"encoding/json/jsontext",
		"encoding/json/v2",
		"github.com/Yamashou/gqlgenc/v3/client",
	}
	referencedPaths := cfg.GQLGenConfig.Models.ReferencedPackages()

	paths := make([]string, 0, len(staticPaths)+len(cfg.GQLGencConfig.TypeAutobind)+len(cfg.GQLGencConfig.FragmentAutobind)+len(referencedPaths))
	paths = append(paths, staticPaths...)
	paths = append(paths, cfg.GQLGencConfig.TypeAutobind...)
	paths = append(paths, cfg.GQLGencConfig.FragmentAutobind...)
	paths = append(paths, referencedPaths...)
	cfg.GQLGenConfig.Packages.LoadAllNames(paths...)
}
