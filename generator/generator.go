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

// outputBackupState is the lifecycle of one output file during Generate.
//
//	backupOutput ──▶ PreviousSaved ──commit──▶ Committed
//	            └──▶ NoPrevious    ──restore─▶ Restored
//
// PreviousSaved and NoPrevious are the unsettled states; each of them moves to
// Committed or Restored exactly once. A commit or restore that fails leaves the
// state unchanged, so that the restore deferred by Generate can still run.
// Committed and Restored are final.
type outputBackupState int

const (
	// outputBackupStatePreviousSaved means the previous output is kept at bakPath.
	outputBackupStatePreviousSaved outputBackupState = iota + 1
	// outputBackupStateNoPrevious means there was no previous output to keep.
	outputBackupStateNoPrevious
	// outputBackupStateCommitted means the result of generation was accepted.
	outputBackupStateCommitted
	// outputBackupStateRestored means the previous output was put back.
	outputBackupStateRestored
)

// isSettled reports whether the backup has been committed or restored.
//
// Arguments:
//   - none
//
// Returns:
//   - bool: true for Committed and Restored
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (s outputBackupState) isSettled() bool {
	return s == outputBackupStateCommitted || s == outputBackupStateRestored
}

// outputBackup is one defined output file moved aside for the duration of Generate.
type outputBackup struct {
	path    string
	bakPath string
	state   outputBackupState
}

// settle moves an unsettled backup to its final state.
//
// Arguments:
//   - finalState: outputBackupStateCommitted or outputBackupStateRestored
//
// Returns:
//   - none
//
// Preconditions:
//   - b is not settled and finalState is a settled state; anything else is a bug and panics
//
// Postconditions:
//   - b.state is finalState, so a later restore leaves the result alone
func (b *outputBackup) settle(finalState outputBackupState) {
	if b.state.isSettled() || !finalState.isSettled() {
		panic(fmt.Sprintf("output backup of %s: invalid transition from state %d to %d", b.path, b.state, finalState))
	}

	b.state = finalState
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
//   - on success, no defined output exists at its output path
//   - a backup left by an interrupted run is kept as the previous output, and
//     the file that run left at the output path is deleted
//   - otherwise a defined output that existed now exists only at its backup path
func backupOutputs(cfg *config.Config) ([]*outputBackup, error) {
	pkgs := []gqlgenconfig.PackageConfig{cfg.Client, cfg.Model}
	backups := make([]*outputBackup, 0, len(pkgs))

	for _, pkg := range pkgs {
		if !pkg.IsDefined() {
			continue
		}

		backup, err := backupOutput(cfg, pkg)
		if err != nil {
			restoreErr := restoreOutputBackups(backups)
			if restoreErr != nil {
				return nil, fmt.Errorf("%w; restoring output: %v", err, restoreErr)
			}

			return nil, err
		}

		backups = append(backups, backup)
	}

	return backups, nil
}

// backupOutput moves pkg.Filename to the same path plus outputBackupSuffix.
//
// A backup is deleted only by a commit or a restore, so one that exists when a
// run starts was left by a run that finished neither. That backup holds the
// output of the last successful run; the file at the output path, if any, was
// written by the unfinished run and may be incomplete.
//
// Arguments:
//   - cfg: the config being generated; its GQLConfig.Packages may be nil
//   - pkg: the output package
//
// Returns:
//   - *outputBackup: the backup of pkg.Filename, non-nil when error is nil
//   - error: non-nil when a leftover backup cannot be checked, the file an
//     interrupted run left at the output path cannot be deleted, or the
//     rename fails for a reason other than the output not existing
//
// Preconditions:
//   - pkg is defined
//   - the working directory is the one pkg.Filename is relative to
//
// Postconditions:
//   - when the cache exists, the import path of pkg's directory is not in it
//   - no file exists at the output path
//   - when a backup was left by an interrupted run, it is kept and treated as
//     the previous output; the file at the output path is deleted
//   - otherwise, when the output existed, it now exists only at the backup path
func backupOutput(cfg *config.Config, pkg gqlgenconfig.PackageConfig) (*outputBackup, error) {
	if cfg.GQLConfig.Packages != nil {
		cfg.GQLConfig.Packages.Evict(pkg.ImportPath())
	}

	backup := &outputBackup{
		path:    pkg.Filename,
		bakPath: pkg.Filename + outputBackupSuffix,
		state:   outputBackupStateNoPrevious,
	}

	_, err := os.Stat(backup.bakPath)
	if err == nil {
		// Renaming the output over the leftover backup would replace the last
		// good output with what the interrupted run wrote.
		err = os.Remove(backup.path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("remove output of an interrupted run: %w", err)
		}

		backup.state = outputBackupStatePreviousSaved

		return backup, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat output backup: %w", err)
	}

	err = os.Rename(backup.path, backup.bakPath)
	if err == nil {
		backup.state = outputBackupStatePreviousSaved

		return backup, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("backup output: %w", err)
	}

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

// restore puts the previous output back, moving an unsettled backup to
// outputBackupStateRestored. A settled backup is left alone.
//
// Arguments:
//   - none
//
// Returns:
//   - error: non-nil when a file cannot be deleted or renamed
//
// Preconditions:
//   - b was returned by backupOutput
//
// Postconditions:
//   - on success b.state is settled: outputBackupStateRestored, or unchanged
//     when it was settled already
//   - on failure b.state is unchanged
func (b *outputBackup) restore() error {
	switch b.state {
	case outputBackupStateCommitted, outputBackupStateRestored:
		return nil
	case outputBackupStateNoPrevious:
		return b.restoreNoPrevious()
	case outputBackupStatePreviousSaved:
		return b.restorePreviousSaved()
	default:
		panic(fmt.Sprintf("output backup of %s: unknown state %d", b.path, b.state))
	}
}

// restoreNoPrevious deletes what generation wrote where there was no previous output.
//
// Arguments:
//   - none
//
// Returns:
//   - error: non-nil when the partial output or a backup path cannot be deleted
//
// Preconditions:
//   - b.state is outputBackupStateNoPrevious
//
// Postconditions:
//   - on success neither b.path nor b.bakPath exists and b.state is outputBackupStateRestored
//   - on failure b.state is unchanged
func (b *outputBackup) restoreNoPrevious() error {
	err := os.Remove(b.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove partial output: %w", err)
	}

	err = os.Remove(b.bakPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove output backup: %w", err)
	}

	b.settle(outputBackupStateRestored)

	return nil
}

// restorePreviousSaved renames the backup onto the original path, replacing a
// partially written output.
//
// Arguments:
//   - none
//
// Returns:
//   - error: non-nil when the backup cannot be checked or renamed
//
// Preconditions:
//   - b.state is outputBackupStatePreviousSaved
//
// Postconditions:
//   - on success b.bakPath does not exist and b.state is outputBackupStateRestored;
//     a backup that is already gone leaves b.path as it is
//   - on failure b.state is unchanged
func (b *outputBackup) restorePreviousSaved() error {
	_, err := os.Stat(b.bakPath)
	if err != nil {
		if os.IsNotExist(err) {
			b.settle(outputBackupStateRestored)

			return nil
		}

		return fmt.Errorf("stat output backup: %w", err)
	}

	// Rename replaces a partially written output.
	err = os.Rename(b.bakPath, b.path)
	if err != nil {
		return fmt.Errorf("restore output: %w", err)
	}

	b.settle(outputBackupStateRestored)

	return nil
}

// outputCommitAction is what committing one output does to its files. It is
// decided for every output before any file is touched.
type outputCommitAction int

const (
	// outputCommitActionUndecided means planCommit could not decide: it is what
	// planCommit returns next to an error. applyCommit rejects it.
	outputCommitActionUndecided outputCommitAction = iota
	// outputCommitActionNone means there is no backup file to deal with: there
	// was no previous output, or the backup is settled already.
	outputCommitActionNone
	// outputCommitActionDropBackup means generation left no file at the output
	// path, so the backup is deleted.
	outputCommitActionDropBackup
	// outputCommitActionKeepPrevious means the generated bytes are identical to
	// the backup, which is renamed back so that the previous mtime is kept.
	outputCommitActionKeepPrevious
	// outputCommitActionKeepGenerated means the generated bytes differ from the
	// backup, which is deleted.
	outputCommitActionKeepGenerated
)

// commitOutputBackups keeps the generated bytes when they differ from the
// backup, and puts the backup back when they are identical so the previous
// mtime is kept. A path that generation left empty drops the backup.
//
// The commit has two phases, so that a failure does not leave some outputs
// committed and others restored: every output is checked and read first, and
// only when all of them passed are the backups deleted or renamed.
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
//   - when an output cannot be checked or read, no backup has been settled and
//     no file has been changed
//   - when deleting or renaming a backup fails, the backups before it in
//     backups are settled and the others are not
func commitOutputBackups(backups []*outputBackup) error {
	actions := make([]outputCommitAction, 0, len(backups))

	for _, backup := range backups {
		action, err := backup.planCommit()
		if err != nil {
			return err
		}

		actions = append(actions, action)
	}

	for i, backup := range backups {
		err := backup.applyCommit(actions[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// planCommit decides what committing the output does, without changing a file.
//
// Arguments:
//   - none
//
// Returns:
//   - outputCommitAction: the action for applyCommit, or
//     outputCommitActionUndecided when error is non-nil
//   - error: non-nil when the generated output or the backup cannot be checked or read
//
// Preconditions:
//   - b was returned by backupOutput
//   - generation has finished
//
// Postconditions:
//   - b.state and the files are unchanged
func (b *outputBackup) planCommit() (outputCommitAction, error) {
	switch b.state {
	case outputBackupStateCommitted, outputBackupStateRestored, outputBackupStateNoPrevious:
		return outputCommitActionNone, nil
	case outputBackupStatePreviousSaved:
	default:
		panic(fmt.Sprintf("output backup of %s: unknown state %d", b.path, b.state))
	}

	_, err := os.Stat(b.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return outputCommitActionUndecided, fmt.Errorf("stat generated output: %w", err)
		}

		// The empty-model deletion left no file.
		return outputCommitActionDropBackup, nil
	}

	newBytes, err := os.ReadFile(b.path)
	if err != nil {
		return outputCommitActionUndecided, fmt.Errorf("read generated output: %w", err)
	}

	oldBytes, err := os.ReadFile(b.bakPath)
	if err != nil {
		return outputCommitActionUndecided, fmt.Errorf("read output backup: %w", err)
	}

	if bytes.Equal(newBytes, oldBytes) {
		return outputCommitActionKeepPrevious, nil
	}

	return outputCommitActionKeepGenerated, nil
}

// applyCommit carries out the action planCommit decided, moving an unsettled
// backup to outputBackupStateCommitted. A settled backup is left alone.
//
// Arguments:
//   - action: the action planCommit returned for b
//
// Returns:
//   - error: non-nil when the backup cannot be deleted or renamed
//
// Preconditions:
//   - action was returned by b.planCommit without an error, and no file
//     changed since
//
// Postconditions:
//   - on success b.state is settled: outputBackupStateCommitted, or unchanged
//     when it was settled already, and b.bakPath does not exist
//   - on failure b.state is unchanged, so restore puts the backup back
func (b *outputBackup) applyCommit(action outputCommitAction) error {
	if b.state.isSettled() {
		return nil
	}

	switch action {
	case outputCommitActionNone:
	case outputCommitActionDropBackup, outputCommitActionKeepGenerated:
		err := os.Remove(b.bakPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove output backup: %w", err)
		}
	case outputCommitActionKeepPrevious:
		// Rename replaces the generated copy, keeping the previous mtime.
		err := os.Rename(b.bakPath, b.path)
		if err != nil {
			return fmt.Errorf("restore unchanged output: %w", err)
		}
	case outputCommitActionUndecided:
		panic(fmt.Sprintf("output backup of %s: commit action was not decided", b.path))
	default:
		panic(fmt.Sprintf("output backup of %s: unknown commit action %d", b.path, action))
	}

	b.settle(outputBackupStateCommitted)

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
		for _, name := range typesUsedByOperation(op) {
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
// for op, in the order they are encountered: the named type of each variable
// first (OperationArguments looks up only that, never nested input fields),
// then the element type of every selected field with an empty selection set,
// which includes an enum or custom scalar used only as a leaf.
//
// Arguments:
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
func typesUsedByOperation(op *ast.OperationDefinition) []string {
	var names []string

	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}

		seen[name] = true
		names = append(names, name)
	}

	// OperationArguments looks up only the named type of each variable.
	for _, v := range op.VariableDefinitions {
		if v.Type != nil {
			add(v.Type.Name())
		}
	}

	var walkSelection func(ss ast.SelectionSet)
	walkSelection = func(ss ast.SelectionSet) {
		for _, sel := range ss {
			switch s := sel.(type) {
			case *ast.Field:
				if s.Definition != nil && s.Definition.Type != nil {
					// A leaf, which includes every enum and custom scalar field.
					if len(s.SelectionSet) == 0 {
						add(s.Definition.Type.Name())
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
