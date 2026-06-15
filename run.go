package main

import (
	"context"
	"fmt"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/introspection"
	"github.com/Yamashou/gqlgenc/v3/plugins"
)

func run(ctx context.Context) error {
	cfgFile, err := config.FindConfigFile(".", []string{".gqlgenc.yml", "gqlgenc.yml", ".gqlgenc.yaml", "gqlgenc.yaml"})
	if err != nil {
		return fmt.Errorf("failed to find config file: %w", err)
	}

	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

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
	return nil
}
