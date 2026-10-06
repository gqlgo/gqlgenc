package generator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"

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

// outputBackupSuffix is not a .go suffix, so packages.Load ignores the backup
// the way it ignored a file that unlinkOutput had removed.
const outputBackupSuffix = ".gqlgenc.bak"

// outputBackup is one defined output file moved aside for the duration of Generate.
type outputBackup struct {
	path    string
	bakPath string
	hadFile bool
	// discard drops the backup without restoring its bytes. Set when success
	// left no file at the output path.
	discard bool
	// settled is set once the backup has been committed or restored, so a
	// later restore does not undo that result.
	settled bool
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

func Generate(ctx context.Context, cfg *config.Config) (err error) {
	// LoadConfig always sets Generate, but callers that build Config by hand may
	// leave it nil. Normalize it once so the plugin and the model hook can rely on it.
	if cfg.Generate == nil {
		cfg.Generate = &config.GenerateConfig{}
	}

	// Move the previous output aside before anything is loaded. A missing file
	// is not an error. When the package cache is shared with an earlier config
	// (GenerateAll), the packages these files belong to are evicted too, so
	// that a cached copy does not keep describing the files just renamed away.
	// gqlgen's templates.Render evicts them again after writing. On failure the
	// backups are put back; on success they are committed after the empty-model
	// file is dropped.
	backups, err := backupOutputs(cfg)
	if err != nil {
		return err
	}

	defer func() {
		restoreErr := restoreOutputBackups(backups)
		if restoreErr == nil {
			return
		}

		if err != nil {
			err = fmt.Errorf("%w; restoring output: %v", err, restoreErr)
			return
		}

		err = fmt.Errorf("restoring output: %w", restoreErr)
	}()

	err = loadSchema(ctx, cfg)
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

	// modelgen fills unbound types when a model file is configured. Without
	// one, a type SourceGenerator.Type will look up must already be bound.
	if !cfg.Model.IsDefined() {
		err = checkClientTypesBound(cfg, operationQueryDocuments)
		if err != nil {
			return err
		}
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

	// onlyUsedModels may empty the model build after gqlgen's empty-build
	// guard, which still writes a package-clause-only models file. Drop that
	// file so the output matches the "no models" case.
	err = removePackageClauseOnlyModel(cfg)
	if err != nil {
		return err
	}

	err = commitOutputBackups(backups)
	if err != nil {
		return err
	}

	return nil
}

// backupOutputs renames each defined output to a backup path and evicts its
// package from the cache when the cache already exists.
//
// Arguments:
//   - cfg: the config being generated; its GQLConfig.Packages may be nil
//
// Returns:
//   - []*outputBackup: one entry per defined output, in client then model order
//   - error: non-nil when a backup cannot be created. Outputs already moved
//     aside are put back, and no backup path is left behind
//
// Preconditions:
//   - the working directory is the one the output filenames are relative to
//
// Postconditions:
//   - on success, each defined output that existed now exists only at its backup path
//   - a defined output that did not exist is unchanged, and a stale backup of it is gone
func backupOutputs(cfg *config.Config) ([]*outputBackup, error) {
	pkgs := []gqlgenconfig.PackageConfig{cfg.Client, cfg.Model}
	backups := make([]*outputBackup, 0, len(pkgs))

	for _, pkg := range pkgs {
		backup, err := backupOutput(cfg, pkg)
		if err != nil {
			restoreErr := restoreOutputBackups(backups)
			if restoreErr != nil {
				return nil, fmt.Errorf("%w; restoring output: %v", err, restoreErr)
			}

			return nil, err
		}

		if backup != nil {
			backups = append(backups, backup)
		}
	}

	return backups, nil
}

// backupOutput moves pkg.Filename to the same path plus outputBackupSuffix.
//
// Arguments:
//   - cfg: the config being generated; its GQLConfig.Packages may be nil
//   - pkg: the output package; ignored when not defined
//
// Returns:
//   - *outputBackup: nil when pkg is not defined
//   - error: non-nil when a stale backup cannot be removed or the rename fails
//     for a reason other than the output not existing
//
// Preconditions:
//   - the working directory is the one pkg.Filename is relative to
//
// Postconditions:
//   - when the cache exists, the import path of pkg's directory is not in it
//   - when the output existed, it now exists only at the backup path
func backupOutput(cfg *config.Config, pkg gqlgenconfig.PackageConfig) (*outputBackup, error) {
	if !pkg.IsDefined() {
		return nil, nil
	}

	if cfg.GQLConfig.Packages != nil {
		cfg.GQLConfig.Packages.Evict(pkg.ImportPath())
	}

	backup := &outputBackup{
		path:    pkg.Filename,
		bakPath: pkg.Filename + outputBackupSuffix,
	}

	err := os.Remove(backup.bakPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale output backup: %w", err)
	}

	err = os.Rename(backup.path, backup.bakPath)
	if err != nil {
		if os.IsNotExist(err) {
			return backup, nil
		}

		return nil, fmt.Errorf("backup output: %w", err)
	}

	backup.hadFile = true

	return backup, nil
}

// restoreOutputBackups puts every unsettled backup back: a partially written
// file is deleted, then the backup is renamed onto the original path.
//
// Arguments:
//   - backups: the outputs moved aside, restored from last to first
//
// Returns:
//   - error: non-nil when a file cannot be deleted or renamed
//
// Preconditions:
//   - none
//
// Postconditions:
//   - each unsettled backup that had a previous file is back at its original path
//   - no unsettled .gqlgenc.bak is left behind
func restoreOutputBackups(backups []*outputBackup) error {
	var errs []error

	for i := len(backups) - 1; i >= 0; i-- {
		err := backups[i].restore()
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// restore deletes a partially written output and puts the backup back.
// A backup that was never created, or that is already settled, is left alone.
func (b *outputBackup) restore() error {
	if b.settled {
		return nil
	}

	if b.discard {
		err := os.Remove(b.bakPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove output backup: %w", err)
		}

		b.settled = true

		return nil
	}

	if !b.hadFile {
		err := os.Remove(b.path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove partial output: %w", err)
		}

		err = os.Remove(b.bakPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove output backup: %w", err)
		}

		b.settled = true

		return nil
	}

	_, err := os.Stat(b.bakPath)
	if err != nil {
		if os.IsNotExist(err) {
			b.settled = true
			return nil
		}

		return fmt.Errorf("stat output backup: %w", err)
	}

	err = os.Remove(b.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove partial output: %w", err)
	}

	err = os.Rename(b.bakPath, b.path)
	if err != nil {
		return fmt.Errorf("restore output: %w", err)
	}

	b.settled = true

	return nil
}

// commitOutputBackups keeps the generated bytes when they differ from the
// backup, and puts the backup back when they are identical so the previous
// mtime is kept. A path that generation left empty drops the backup.
//
// Arguments:
//   - backups: the outputs moved aside
//
// Returns:
//   - error: non-nil when a backup cannot be committed. That backup stays
//     unsettled so the caller can restore it
//
// Preconditions:
//   - generation has finished, including removePackageClauseOnlyModel
//
// Postconditions:
//   - on success every backup is settled and no .gqlgenc.bak remains
func commitOutputBackups(backups []*outputBackup) error {
	for _, backup := range backups {
		err := backup.commit()
		if err != nil {
			return err
		}
	}

	return nil
}

// commit settles one backup after a successful generate.
func (b *outputBackup) commit() error {
	if !b.hadFile {
		b.settled = true
		return nil
	}

	_, err := os.Stat(b.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat generated output: %w", err)
		}

		// The empty-model deletion left no file. Drop the backup and do not
		// restore the previous bytes.
		b.discard = true

		err = os.Remove(b.bakPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove output backup: %w", err)
		}

		b.settled = true

		return nil
	}

	newBytes, err := os.ReadFile(b.path)
	if err != nil {
		return fmt.Errorf("read generated output: %w", err)
	}

	oldBytes, err := os.ReadFile(b.bakPath)
	if err != nil {
		return fmt.Errorf("read output backup: %w", err)
	}

	if bytes.Equal(newBytes, oldBytes) {
		err = os.Remove(b.path)
		if err != nil {
			return fmt.Errorf("replace unchanged output: %w", err)
		}

		err = os.Rename(b.bakPath, b.path)
		if err != nil {
			return fmt.Errorf("restore unchanged output: %w", err)
		}

		b.settled = true

		return nil
	}

	err = os.Remove(b.bakPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove output backup: %w", err)
	}

	b.settled = true

	return nil
}

// checkClientTypesBound reports the first type that client generation would
// look up and that is not bound to a Go type.
//
// Arguments:
//   - cfg: the config, after GQLConfig.Init
//   - docs: one query document per operation, in document order
//
// Returns:
//   - error: non-nil for the first unbound type
//
// Preconditions:
//   - cfg.Model is not defined, so modelgen will not fill these types
//   - cfg.GQLConfig.Init has bound the built-in scalars
//
// Postconditions:
//   - none
func checkClientTypesBound(cfg *config.Config, docs []*ast.QueryDocument) error {
	bound := map[string]bool{}

	for _, doc := range docs {
		if len(doc.Operations) == 0 {
			continue
		}

		op := doc.Operations[0]
		for _, name := range typesUsedByOperation(cfg.GQLConfig.Schema, op) {
			if bound[name] {
				continue
			}

			if len(cfg.GQLConfig.Models[name].Model) > 0 {
				bound[name] = true
				continue
			}

			return unboundTypeError(name, op.Name)
		}
	}

	return nil
}

// unboundTypeError is the binding failure for typeName as used by the operation
// named opName. An empty opName is an anonymous operation.
func unboundTypeError(typeName, opName string) error {
	const hint = "add it to `autobind`/`models`, or set `model.filename` so gqlgenc generates it"

	if opName == "" {
		return fmt.Errorf("type %q is used by an anonymous operation but is not bound to a Go type; %s", typeName, hint)
	}

	return fmt.Errorf("type %q is used by operation %q but is not bound to a Go type; %s", typeName, opName, hint)
}

// typesUsedByOperation returns the type names SourceGenerator.Type looks up
// for op, in the order they are encountered: variable and nested input types
// first (as CollectTypesFromQueryDocuments collects them), then the element
// type of every selected field with an empty selection set, which includes a
// custom scalar used only as a leaf.
//
// Arguments:
//   - schema: the loaded schema
//   - op: one operation
//
// Returns:
//   - []string: type names, without duplicates, in encounter order
//
// Preconditions:
//   - op's selections are resolved
//
// Postconditions:
//   - none
func typesUsedByOperation(schema *ast.Schema, op *ast.OperationDefinition) []string {
	var names []string

	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}

		seen[name] = true
		names = append(names, name)
	}

	processed := map[string]bool{}

	var walkInput func(def *ast.Definition)
	walkInput = func(def *ast.Definition) {
		if def == nil || processed[def.Name] {
			return
		}

		processed[def.Name] = true
		add(def.Name)

		for _, field := range def.Fields {
			if field.Type == nil {
				continue
			}

			typeName := field.Type.Name()
			add(typeName)

			if fieldDef, ok := schema.Types[typeName]; ok && fieldDef.IsInputType() {
				walkInput(fieldDef)
			}
		}
	}

	for _, v := range op.VariableDefinitions {
		addTypeReference(v.Type, add)

		if v.Type == nil {
			continue
		}

		typeName := v.Type.Name()
		if def, ok := schema.Types[typeName]; ok && def.IsInputType() {
			walkInput(def)
		}
	}

	var walkSelection func(ss ast.SelectionSet)
	walkSelection = func(ss ast.SelectionSet) {
		for _, sel := range ss {
			switch s := sel.(type) {
			case *ast.Field:
				if s.Definition != nil && s.Definition.Type != nil {
					typeName := s.Definition.Type.Name()
					if def, ok := schema.Types[typeName]; ok && def.Kind == ast.Enum {
						add(typeName)
					}

					if len(s.SelectionSet) == 0 {
						add(typeName)
					}
				}

				walkSelection(s.SelectionSet)
			case *ast.InlineFragment:
				walkSelection(s.SelectionSet)
			case *ast.FragmentSpread:
				if s.Definition != nil {
					walkSelection(s.Definition.SelectionSet)
				}
			}
		}
	}

	walkSelection(op.SelectionSet)

	return names
}

// addTypeReference adds every named type in t, matching
// querydocument.collectTypeFromTypeReference.
func addTypeReference(t *ast.Type, add func(string)) {
	if t == nil {
		return
	}

	if t.NamedType != "" {
		add(t.NamedType)
	}

	addTypeReference(t.Elem, add)
}

// removePackageClauseOnlyModel deletes cfg.Model.Filename when it exists and
// contains no declarations (package clause only), which happens when
// onlyUsedModels filters away every model after gqlgen already decided to
// render. Evicts the package from the gqlgen cache when present.
func removePackageClauseOnlyModel(cfg *config.Config) error {
	if !cfg.Model.IsDefined() {
		return nil
	}
	if !isPackageClauseOnly(cfg.Model.Filename) {
		return nil
	}
	err := os.Remove(cfg.Model.Filename)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove empty model file: %w", err)
	}
	if cfg.GQLConfig.Packages != nil {
		cfg.GQLConfig.Packages.Evict(cfg.Model.ImportPath())
	}
	return nil
}

func isPackageClauseOnly(filename string) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return false
	}
	return len(f.Decls) == 0
}
