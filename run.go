package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/plugins"
	"github.com/Yamashou/gqlgenc/v3/queryparser"
)

var configFilenames = []string{".gqlgenc.yml", "gqlgenc.yml", ".gqlgenc.yaml", "gqlgenc.yaml"}

func run(ctx context.Context, configFiles ...string) error {
	if len(configFiles) == 0 {
		cfgFile, err := config.FindConfigFile(".", configFilenames)
		if err != nil {
			return fmt.Errorf("failed to find config file: %w", err)
		}
		configFiles = []string{cfgFile}
	}

	// 処理中に設定ファイルのディレクトリへ chdir するため、後続の相対パスが
	// 解決できなくなる前にすべて絶対パスへ変換しておく。
	for i, configFile := range configFiles {
		absPath, err := filepath.Abs(configFile)
		if err != nil {
			return fmt.Errorf("failed to resolve config path %s: %w", configFile, err)
		}
		configFiles[i] = absPath
	}

	// 複数の設定を1プロセスで処理するときは、パッケージキャッシュ (go list と型検査の結果) を
	// 設定の間で使い回して再ロードを避ける。キャッシュはスレッドセーフではないため直列に処理する。
	cache := newPackageCache()
	for _, configFile := range configFiles {
		if err := generate(ctx, configFile, cache); err != nil {
			if len(configFiles) > 1 {
				return fmt.Errorf("%s: %w", configFile, err)
			}
			return err
		}
	}

	return nil
}

// generate は、設定ファイル 1 つの出力を生成する。
func generate(ctx context.Context, configFile string, cache *packageCache) error {
	// 設定内の相対パスは設定ファイルのディレクトリを基準に解決する
	if err := os.Chdir(filepath.Dir(configFile)); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 前回の出力を、何もロードしないうちに出力先から退避する。出力先のパッケージを束縛先として
	// ロードするときに、前回の生成物の型を拾わせないためである。消さずに退避するのは、スキーマや
	// クエリの誤りで生成が失敗したときに、前回の出力を戻すためである (gqlgo/gqlgenc#322)。
	backups, err := backupOutputs(cfg, cache)
	if err != nil {
		return fmt.Errorf("failed to back up outputs: %w", err)
	}
	// panic で抜けるときも前回の出力を戻す。正常に終えたときとエラーで戻したときは、退避がすべて
	// 確定済みなので何も変えない。
	defer func() {
		_ = restoreOutputBackups(backups)
	}()

	if err := generateOutputs(ctx, cfg, cache, backups); err != nil {
		if restoreErr := restoreOutputBackups(backups); restoreErr != nil {
			return fmt.Errorf("%w; failed to restore outputs: %w", err, restoreErr)
		}

		return err
	}

	// 生成したパッケージをキャッシュから外す。後続の設定が同じパッケージを束縛する場合に、
	// 生成後の状態で読み込み直させる。
	for _, output := range outputConfigs(cfg) {
		cache.evict(output)
	}

	return nil
}

// generateOutputs は、スキーマとクエリを読み込んでコードを生成し、生成の結果を採用する。
//
// 事前条件: backups は backupOutputs(cfg, cache) が返した退避で、出力先に前回の出力は無い。
// 事後条件: 成功したら backups はすべて確定している。失敗したら、未確定の退避が残っていて、
// 呼び出し側が restoreOutputBackups で前回の出力を戻せる。
func generateOutputs(ctx context.Context, cfg *config.Config, cache *packageCache, backups []*outputBackup) error {
	schema, err := loadSchema(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	// 束縛の失敗 (スキーマの検証・束縛先のパッケージの読み込み) も、スキーマの読み込みの失敗として報告する
	gqlgenCfg, err := bindTypes(cfg, schema, cache)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	queryDocument, operationQueryDocuments, err := queryparser.Load(schema, cfg.Query)
	if err != nil {
		return fmt.Errorf("failed to load query: %w", err)
	}

	if err := plugins.GenerateCode(cfg, gqlgenCfg, cache.binder, queryDocument, operationQueryDocuments); err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	// 使用フィルタがすべての型を除いたときに残る、宣言の無い model ファイルを消す
	if err := removePackageClauseOnlyModel(cfg, cache); err != nil {
		return err
	}

	// 読めないコードを出力として採用しない (前回の出力へ戻す)
	if err := validateOutputs(cfg, cache); err != nil {
		return err
	}

	if err := commitOutputBackups(backups); err != nil {
		return fmt.Errorf("failed to commit outputs: %w", err)
	}

	return nil
}

// outputConfigs は、設定された出力 (model・query・client) の出力先を返す。指定していない出力は含めない。
func outputConfigs(cfg *config.Config) []*gqlgenconfig.PackageConfig {
	outputs := make([]*gqlgenconfig.PackageConfig, 0, 3)
	for _, output := range []*gqlgenconfig.PackageConfig{&cfg.ModelGen, &cfg.QueryGen, &cfg.ClientGen} {
		if output.IsDefined() {
			outputs = append(outputs, output)
		}
	}

	return outputs
}
