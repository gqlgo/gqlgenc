package main

import (
	"context"
	"fmt"
	"net/http"

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

	// リモート(endpoint)指定時は config を読み取って introspection でスキーマを取得し、
	// config に設定する。これにより config パッケージは client に依存しない。
	if endpoint := cfg.GQLGencConfig.Endpoint; endpoint != nil {
		schema, err := introspection.LoadRemoteSchema(ctx, endpoint.URL, http.Header(endpoint.Headers))
		if err != nil {
			return fmt.Errorf("failed to introspect schema: %w", err)
		}
		cfg.GQLGenConfig.Schema = schema
	}

	if err := cfg.LoadSchema(); err != nil {
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
