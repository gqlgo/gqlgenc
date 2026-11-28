package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/introspection"
	"github.com/Yamashou/gqlgenc/v3/plugins"
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

	// 複数の設定を1プロセスで処理するときは、gqlgen のパッケージキャッシュ
	// (go list と型検査の結果) を config 間で使い回して再ロードを避ける。
	// キャッシュはスレッドセーフではないため直列に処理する。
	var prevCfg *config.Config
	for _, configFile := range configFiles {
		cfg, err := generate(ctx, configFile, prevCfg)
		if err != nil {
			if len(configFiles) > 1 {
				return fmt.Errorf("%s: %w", configFile, err)
			}
			return err
		}
		prevCfg = cfg
	}

	return nil
}

func generate(ctx context.Context, configFile string, prevCfg *config.Config) (*config.Config, error) {
	// 設定内の相対パスは設定ファイルのディレクトリを基準に解決する
	if err := os.Chdir(filepath.Dir(configFile)); err != nil {
		return nil, fmt.Errorf("failed to change directory: %w", err)
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 直前の config が構築したパッケージキャッシュを引き継ぐ。gqlgen の Init() は
	// Packages が nil のときだけ新規作成するため、これで go list + 型検査が1回で済む。
	// 生成で書き換わったパッケージは templates.Render が Evict するため共有しても安全。
	if prevCfg != nil {
		cfg.GQLGenConfig.Packages = prevCfg.GQLGenConfig.Packages
		cfg.GQLGencConfig.TypeBinder = prevCfg.GQLGencConfig.TypeBinder
	}

	// 前回の出力を、何もロードしないうちに出力先から退避する。出力先のパッケージを束縛先として
	// ロードするときに、前回の生成物の型を拾わせないためである。消さずに退避するのは、スキーマや
	// クエリの誤りで生成が失敗したときに、前回の出力を戻すためである (gqlgo/gqlgenc#322)。
	backups, err := backupOutputs(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to back up outputs: %w", err)
	}
	// panic で抜けるときも前回の出力を戻す。正常に終えたときとエラーで戻したときは、退避がすべて
	// 確定済みなので何も変えない。
	defer func() {
		_ = restoreOutputBackups(backups)
	}()

	if err := generateOutputs(ctx, cfg, backups); err != nil {
		if restoreErr := restoreOutputBackups(backups); restoreErr != nil {
			return nil, fmt.Errorf("%w; failed to restore outputs: %w", err, restoreErr)
		}

		return nil, err
	}

	// 生成したパッケージを軽量 binder のキャッシュから外す。後続の config が
	// 同じパッケージを束縛する場合に、生成後の状態で再ロードさせる。
	for _, filename := range []string{cfg.GQLGenConfig.Model.Filename, cfg.GQLGencConfig.QueryGen.Filename, cfg.GQLGencConfig.ClientGen.Filename} {
		if filename == "" {
			continue
		}
		if dir, err := filepath.Abs(filepath.Dir(filename)); err == nil {
			cfg.GQLGencConfig.TypeBinder.EvictDir(dir)
		}
	}

	return cfg, nil
}

// generateOutputs は、スキーマとクエリを読み込んでコードを生成し、生成の結果を採用する。
//
// 事前条件: backups は backupOutputs(cfg) が返した退避で、出力先に前回の出力は無い。
// 事後条件: 成功したら backups はすべて確定している。失敗したら、未確定の退避が残っていて、
// 呼び出し側が restoreOutputBackups で前回の出力を戻せる。
func generateOutputs(ctx context.Context, cfg *config.Config, backups []*outputBackup) error {
	// introspection.LoadRemoteSchema を注入する。endpoint 指定時のリモート取得は
	// LoadSchema の中でこれを呼ぶが、config パッケージ自体は client に依存しない。
	if err := cfg.LoadSchema(ctx, introspection.LoadRemoteSchema); err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	if err := cfg.GQLGencConfig.LoadQuery(cfg.GQLGenConfig.Schema); err != nil {
		return fmt.Errorf("failed to load query: %w", err)
	}

	if err := plugins.GenerateCode(cfg); err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	// 使用フィルタがすべての型を除いたときに残る、宣言の無い model ファイルを消す
	if err := removePackageClauseOnlyModel(cfg); err != nil {
		return err
	}

	if err := commitOutputBackups(backups); err != nil {
		return fmt.Errorf("failed to commit outputs: %w", err)
	}

	return nil
}
