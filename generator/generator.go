package generator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"syscall"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/plugin"
	"github.com/99designs/gqlgen/plugin/federation"
	"github.com/99designs/gqlgen/plugin/modelgen"
	"github.com/gqlgo/gqlgenc/clientgenv2"
	"github.com/gqlgo/gqlgenc/config"
	"github.com/gqlgo/gqlgenc/parsequery"
	"github.com/gqlgo/gqlgenc/querydocument"
)

func mutateHook(cfg *config.Config, usedTypes map[string]bool) func(b *modelgen.ModelBuild) *modelgen.ModelBuild {
	return func(build *modelgen.ModelBuild) *modelgen.ModelBuild {
		// only generate used models
		if cfg.Generate.OnlyUsedModels != nil && *cfg.Generate.OnlyUsedModels {
			var newModels []*modelgen.Object

			for _, model := range build.Models {
				if usedTypes[model.Name] {
					newModels = append(newModels, model)
				}
			}

			build.Models = newModels
			build.Interfaces = nil
		}

		return build
	}
}

func Generate(ctx context.Context, cfg *config.Config) error {
	_ = syscall.Unlink(cfg.Client.Filename)
	if cfg.Model.IsDefined() {
		_ = syscall.Unlink(cfg.Model.Filename)
	}

	if cfg.Federation.Version != 0 {
		var (
			fedPlugin plugin.Plugin
			err       error
		)

		fedPlugin, err = federation.New(cfg.Federation.Version, cfg.GQLConfig)
		if err != nil {
			return fmt.Errorf("failed to create federation plugin: %w", err)
		}

		if fed, ok := fedPlugin.(plugin.EarlySourcesInjector); ok {
			sources, err := fed.InjectSourcesEarly()
			if err == nil {
				cfg.GQLConfig.Sources = append(cfg.GQLConfig.Sources, sources...)
			} else {
				return fmt.Errorf("failed to inject federation directives: %w", err)
			}
		} else if fed, ok := fedPlugin.(plugin.EarlySourceInjector); ok {
			if source := fed.InjectSourceEarly(); source != nil {
				cfg.GQLConfig.Sources = append(cfg.GQLConfig.Sources, source)
			}
		} else {
			return errors.New("failed to inject federation directives")
		}
	}

	err := cfg.LoadSchema(ctx)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	err = cfg.GQLConfig.Init()
	if err != nil {
		return fmt.Errorf("generating core failed: %w", err)
	}

	// sort Implements to ensure a deterministic output
	for _, v := range cfg.GQLConfig.Schema.Implements {
		sort.Slice(v, func(i, j int) bool { return v[i].Name < v[j].Name })
	}

	querySources, err := parsequery.LoadQuerySources(cfg.Query)
	if err != nil {
		return fmt.Errorf("load query sources failed: %w", err)
	}

	queryDocument, err := parsequery.ParseQueryDocuments(cfg.GQLConfig.Schema, querySources)
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	operationQueryDocuments, err := querydocument.QueryDocumentsByOperations(cfg.GQLConfig.Schema, queryDocument.Operations)
	if err != nil {
		return fmt.Errorf(": %w", err)
	}

	var clientGen api.Option
	if cfg.Generate != nil {
		clientGen = api.AddPlugin(clientgenv2.New(queryDocument, operationQueryDocuments, cfg.Client, cfg.Generate))
	}

	var plugins []plugin.Plugin

	if cfg.Model.IsDefined() {
		usedTypes := querydocument.CollectTypesFromQueryDocuments(cfg.GQLConfig.Schema, operationQueryDocuments)
		p := &modelgen.Plugin{
			MutateHook: mutateHook(cfg, usedTypes),
			FieldHook:  modelgen.DefaultFieldMutateHook,
		}

		plugins = append(plugins, p)
	}

	clientGen(cfg.GQLConfig, &plugins)

	for _, p := range plugins {
		if mut, ok := p.(plugin.ConfigMutator); ok {
			err := mut.MutateConfig(cfg.GQLConfig)
			if err != nil {
				return fmt.Errorf("%s failed: %w", p.Name(), err)
			}
		}
	}

	return nil
}
