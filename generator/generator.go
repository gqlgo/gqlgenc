package generator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"syscall"

	"github.com/99designs/gqlgen/api"
	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin"
	"github.com/99designs/gqlgen/plugin/federation"
	"github.com/99designs/gqlgen/plugin/modelgen"

	"github.com/gqlgo/gqlgenc/clientgenv2"
	"github.com/gqlgo/gqlgenc/config"
	"github.com/gqlgo/gqlgenc/parsequery"
	"github.com/gqlgo/gqlgenc/querydocument"

	"github.com/vektah/gqlparser/v2/ast"
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

			var newEnums []*modelgen.Enum
			for _, enum := range build.Enums {
				if usedTypes[enum.Name] {
					newEnums = append(newEnums, enum)
				}
			}
			build.Enums = newEnums

			build.Interfaces = nil
		}

		return build
	}
}

// unlinkOutput removes the file of an output package and drops that package
// from the gqlgen package cache when the cache already exists.
//
// Arguments:
//   - cfg: the config being generated; its GQLConfig.Packages may be nil
//   - pkg: the output package (client or model); ignored when not defined
//
// Returns:
//   - nothing
//
// Preconditions:
//   - the working directory is the one pkg.Filename is relative to
//
// Postconditions:
//   - pkg.Filename does not exist
//   - when the cache exists, the import path of pkg's directory is not in it
func unlinkOutput(cfg *config.Config, pkg gqlgenconfig.PackageConfig) {
	if !pkg.IsDefined() {
		return
	}

	_ = syscall.Unlink(pkg.Filename)

	if cfg.GQLConfig.Packages != nil {
		cfg.GQLConfig.Packages.Evict(pkg.ImportPath())
	}
}

// loadSchema injects the federation directives, if any, and loads the schema
// from the local files or the remote endpoint into cfg.GQLConfig.Schema.
//
// Arguments:
//   - ctx: used for the remote schema introspection, if any
//   - cfg: the config to load the schema of
//
// Returns:
//   - error: non-nil when the federation plugin or the schema fails to load
//
// Preconditions:
//   - the working directory is the one the schema paths are relative to
//
// Postconditions:
//   - on success cfg.GQLConfig.Schema is non-nil, with the relative import
//     paths of its @goModel and @goEnum directives replaced by full ones
func loadSchema(ctx context.Context, cfg *config.Config) error {
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
			if err != nil {
				return fmt.Errorf("failed to inject federation directives: %w", err)
			}

			cfg.GQLConfig.Sources = append(cfg.GQLConfig.Sources, sources...)
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

	normalizeSchemaImportPaths(cfg.GQLConfig.Schema)

	return nil
}

func Generate(ctx context.Context, cfg *config.Config) error {
	// LoadConfig always sets Generate, but callers that build Config by hand may
	// leave it nil. Normalize it once so the plugin and the model hook can rely on it.
	if cfg.Generate == nil {
		cfg.Generate = &config.GenerateConfig{}
	}

	// Remove the previous output before anything is loaded. When the package
	// cache is shared with an earlier config (GenerateAll), the packages these
	// files belong to are evicted too, so that a cached copy does not keep
	// describing the files just removed. gqlgen's templates.Render evicts them
	// again after writing.
	unlinkOutput(cfg, cfg.Client)
	unlinkOutput(cfg, cfg.Model)

	err := loadSchema(ctx, cfg)
	if err != nil {
		return err
	}

	err = cfg.GQLConfig.Init()
	if err != nil {
		return fmt.Errorf("generating core failed: %w", err)
	}

	// sort Implements to ensure a deterministic output
	for _, v := range cfg.GQLConfig.Schema.Implements {
		slices.SortFunc(v, func(a, b *ast.Definition) int { return strings.Compare(a.Name, b.Name) })
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

	clientGen := api.AddPlugin(clientgenv2.New(queryDocument, operationQueryDocuments, cfg.Client, cfg.Generate))

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
