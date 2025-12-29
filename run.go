package main

import (
	"context"
	"fmt"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/plugins"
)

func run() error {
	cfgFile, err := config.FindConfigFile(".", []string{".gqlgenc.yml", "gqlgenc.yml", ".gqlgenc.yaml", "gqlgenc.yaml"})
	if err != nil {
		return fmt.Errorf("failed to find config file: %w", err)
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	ctx := context.Background()
	if err := cfg.PrepareSchema(ctx); err != nil {
		return fmt.Errorf("failed to prepare schema: %w", err)
	}

	if err := plugins.GenerateCode(cfg); err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}
	return nil
}
