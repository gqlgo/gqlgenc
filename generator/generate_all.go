package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/gqlgo/gqlgenc/config"

	"github.com/vektah/gqlparser/v2/ast"
)

// LoadConfigError is returned by GenerateAll when a config could not be found
// or loaded, as opposed to a failure while generating code from it.
type LoadConfigError struct {
	// ConfigDir is the directory given by the caller.
	ConfigDir string
	// Err is the underlying error.
	Err error
}

func (e *LoadConfigError) Error() string {
	return e.Err.Error()
}

func (e *LoadConfigError) Unwrap() error {
	return e.Err
}

// GenerateAll runs Generate for every config directory, in order, in a single
// process, sharing gqlgen's package cache between them: the packages named in
// autobind and models are loaded and type checked once per run instead of
// once per config.
//
// Each config is processed with the working directory set to the directory of
// its config file, so the paths in a config are relative to the config file.
// The cache is not safe for concurrent use, so the configs are processed
// serially; run several processes to use more cores.
//
// The configs must belong to the same Go module, because the cache is keyed
// by import path and resolved through the module of the first config. gqlgen
// keeps the Go names it has chosen for GraphQL names for the whole process,
// so the configs should share one schema: with different schemas a GraphQL
// name whose Go name is already taken gets a numeric suffix that a separate
// run would not add. A package named in autobind or models must not import
// the output of another config of the same run, because only the output
// packages themselves are dropped from the cache when they are rewritten.
//
// Arguments:
//   - ctx: used for the remote schema introspection, if any
//   - configDirs: the directories holding a config file; "." when empty. The
//     config file is searched upward from each directory, like the -c flag.
//     A config file given more than once is generated once
//
// Returns:
//   - error: the first failure. It is a *LoadConfigError when the config could
//     not be found or loaded. When more than one directory is given, the
//     message is prefixed with the directory so the failing config can be
//     identified
//
// Preconditions:
//   - see above
//
// Postconditions:
//   - the working directory is restored before returning
//   - a config is never started after an earlier one failed
func GenerateAll(ctx context.Context, configDirs []string) error {
	if len(configDirs) == 0 {
		configDirs = []string{"."}
	}

	originalWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	defer func() { _ = os.Chdir(originalWD) }()

	// Resolve every directory before the first chdir, because loading a
	// config changes the working directory and a relative directory given
	// after it would otherwise be resolved against the previous config.
	absDirs := make([]string, 0, len(configDirs))

	for i, dir := range configDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return wrapConfigError(configDirs, i, &LoadConfigError{ConfigDir: dir, Err: fmt.Errorf("failed to resolve config directory: %w", err)})
		}

		absDirs = append(absDirs, absDir)
	}

	// packages is the cache of the first config, handed to the others. The
	// zero value of a gqlgen config gives a variable of the cache type, which
	// lives in gqlgen's internal package and cannot be named here. gqlgen's
	// Init creates a cache only when Packages is nil.
	packages := new(gqlgenconfig.Config).Packages
	seen := map[string]bool{}

	for i, absDir := range absDirs {
		cfgFile, err := config.FindConfigFile(absDir)
		if err != nil {
			return wrapConfigError(configDirs, i, &LoadConfigError{ConfigDir: configDirs[i], Err: err})
		}

		if seen[cfgFile] {
			continue
		}

		seen[cfgFile] = true

		cfg, err := loadConfig(cfgFile)
		if err != nil {
			return wrapConfigError(configDirs, i, &LoadConfigError{ConfigDir: configDirs[i], Err: err})
		}

		cfg.GQLConfig.Packages = packages

		err = Generate(ctx, cfg)
		if err != nil {
			return wrapConfigError(configDirs, i, err)
		}

		packages = cfg.GQLConfig.Packages
	}

	return nil
}

// wrapConfigError prefixes err with the config directory when more than one
// config was given, so the failing config can be identified. With a single
// config the error is returned unchanged, as before the -c flag could be
// repeated.
//
// Arguments:
//   - configDirs: the directories given by the caller
//   - i: the index of the failing config in configDirs
//   - err: the failure
//
// Returns:
//   - error: err, prefixed with configDirs[i] when len(configDirs) > 1
//
// Preconditions:
//   - 0 <= i < len(configDirs)
//
// Postconditions:
//   - errors.As and errors.Is still find err in the result
func wrapConfigError(configDirs []string, i int, err error) error {
	if len(configDirs) > 1 {
		return fmt.Errorf("%s: %w", configDirs[i], err)
	}

	return err
}

// loadConfig changes the working directory to the directory of cfgFile and
// loads it. Relative import paths in autobind and models are replaced with
// full import paths, because gqlgen's package cache only recognizes a
// package it has loaded by its full import path (or absolute directory), so a
// relative one would be listed and type checked again by every config.
//
// Arguments:
//   - cfgFile: the path of the config file
//
// Returns:
//   - *config.Config: the loaded config
//   - error: non-nil when the directory cannot be entered or the config does
//     not parse
//
// Preconditions:
//   - cfgFile exists
//
// Postconditions:
//   - on success the working directory is the directory of the config file
func loadConfig(cfgFile string) (*config.Config, error) {
	err := os.Chdir(filepath.Dir(cfgFile))
	if err != nil {
		return nil, fmt.Errorf("failed to enter config directory: %w", err)
	}

	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		return nil, err
	}

	normalizeImportPaths(cfg.GQLConfig)

	return cfg, nil
}

// normalizeImportPaths replaces the relative import paths (".", "./pkg",
// "../pkg") in autobind and models with the full import path of that
// directory, as
// gqlgen itself computes it for the output packages. An entry whose import
// path cannot be determined (a directory outside any module) is left as it
// is.
//
// Arguments:
//   - cfg: the gqlgen config to rewrite in place
//
// Returns:
//   - nothing
//
// Preconditions:
//   - the working directory is the one the relative paths are relative to
//
// Postconditions:
//   - the generated code is the same as with the relative paths, because
//     gqlgen records bound types by the import path of their package anyway
func normalizeImportPaths(cfg *gqlgenconfig.Config) {
	for i, pkg := range cfg.AutoBind {
		cfg.AutoBind[i] = fullImportPath(pkg)
	}

	for _, entry := range cfg.Models {
		for i, model := range entry.Model {
			entry.Model[i] = fullModelPath(model)
		}
	}
}

// fullModelPath returns model with its package part replaced by the full
// import path when it is relative.
//
// Arguments:
//   - model: a model reference such as "./pkg.Type"
//
// Returns:
//   - string: the reference with a full import path, or model when its
//     package part is not relative or has no dot
//
// Preconditions:
//   - the working directory is the one the relative path is relative to
//
// Postconditions:
//   - none
func fullModelPath(model string) string {
	pkg := packageOfModel(model)
	if pkg == model {
		return model
	}

	return fullImportPath(pkg) + model[len(pkg):]
}

// normalizeSchemaImportPaths replaces the relative import paths in the
// @goModel and @goEnum directives of schema with full import paths, for the
// same reason as normalizeImportPaths: gqlgen adds them to models during
// Init, and the package cache only recognizes full import paths.
//
// Arguments:
//   - schema: the loaded schema, rewritten in place
//
// Returns:
//   - nothing
//
// Preconditions:
//   - the working directory is the one the relative paths are relative to
//
// Postconditions:
//   - no @goModel model or @goEnum value in schema names a relative package
//     whose import path can be determined
func normalizeSchemaImportPaths(schema *ast.Schema) {
	forEachSchemaModel(schema, func(model *string) {
		*model = fullModelPath(*model)
	})
}

// forEachSchemaModel calls fn with every model reference the @goModel
// (model, models) and @goEnum (value) directives of schema hold, so that it
// can be read or rewritten in place.
//
// Arguments:
//   - schema: the loaded schema
//   - fn: called with a pointer to each reference
//
// Returns:
//   - nothing
//
// Preconditions:
//   - none
//
// Postconditions:
//   - fn was called once per reference
func forEachSchemaModel(schema *ast.Schema, fn func(model *string)) {
	for _, def := range schema.Types {
		for _, directive := range def.Directives {
			if directive.Name != "goModel" {
				continue
			}

			for _, arg := range directive.Arguments {
				switch arg.Name {
				case "model":
					fn(&arg.Value.Raw)
				case "models":
					for _, child := range arg.Value.Children {
						fn(&child.Value.Raw)
					}
				}
			}
		}

		for _, value := range def.EnumValues {
			for _, directive := range value.Directives {
				if directive.Name != "goEnum" {
					continue
				}

				for _, arg := range directive.Arguments {
					if arg.Name == "value" {
						fn(&arg.Value.Raw)
					}
				}
			}
		}
	}
}

// fullImportPath returns the import path of a relative package directory, or
// pkg itself when pkg is not relative or its import path is unknown.
//
// Arguments:
//   - pkg: an import path, possibly relative (".", "./pkg", "../pkg")
//
// Returns:
//   - string: the full import path, or pkg
//
// Preconditions:
//   - the working directory is the one pkg is relative to
//
// Postconditions:
//   - none
func fullImportPath(pkg string) string {
	if pkg != "." && pkg != ".." && !strings.HasPrefix(pkg, "./") && !strings.HasPrefix(pkg, "../") {
		return pkg
	}

	// PackageConfig.ImportPath derives the import path of the directory of a
	// file from the enclosing go.mod; the file itself need not exist.
	importPath := (&gqlgenconfig.PackageConfig{Filename: filepath.Join(pkg, "_.go")}).ImportPath()
	if importPath == "" {
		return pkg
	}

	return importPath
}

// packageOfModel returns the import path part of a model reference such as
// "github.com/x/y/pkg.Type".
//
// Arguments:
//   - model: the model reference
//
// Returns:
//   - string: the part before the last dot, or model when it has no dot
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func packageOfModel(model string) string {
	dot := strings.LastIndex(model, ".")
	if dot < 0 {
		return model
	}

	return model[:dot]
}
