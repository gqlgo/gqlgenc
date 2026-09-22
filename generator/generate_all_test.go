package generator_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/gqlgo/gqlgenc/generator"
)

const (
	multiConfigRoot   = "testdata/multi_config"
	multiConfigModels = "github.com/gqlgo/gqlgenc/generator/testdata/multi_config/models"
)

// multiConfigNames are the config directories of the multi_config fixture,
// relative to its root, in the order they must be generated: b before c and
// d (see the README of the fixture).
var multiConfigNames = []string{"a", "b", "c", "d"}

// multiConfigDirs returns the absolute config directories of the fixture in
// generation order.
func multiConfigDirs(t *testing.T) []string {
	t.Helper()

	root, err := filepath.Abs(multiConfigRoot)
	require.NoError(t, err)

	dirs := make([]string, 0, len(multiConfigNames))
	for _, name := range multiConfigNames {
		dirs = append(dirs, filepath.Join(root, name))
	}

	return dirs
}

// removeMultiConfigOutputs deletes every actual directory of the fixture.
func removeMultiConfigOutputs(t *testing.T) {
	t.Helper()

	for _, dir := range multiConfigDirs(t) {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, "actual")))
	}
}

// readMultiConfigOutputs returns the generated files keyed by their path
// relative to the fixture root.
func readMultiConfigOutputs(t *testing.T) map[string]string {
	t.Helper()

	root, err := filepath.Abs(multiConfigRoot)
	require.NoError(t, err)

	files := map[string]string{}

	for _, dir := range multiConfigDirs(t) {
		actual := filepath.Join(dir, "actual")

		_, err := os.Stat(actual)
		if os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(actual)
		require.NoError(t, err)

		for _, entry := range entries {
			path := filepath.Join(actual, entry.Name())

			content, err := os.ReadFile(path)
			require.NoError(t, err)

			rel, err := filepath.Rel(root, path)
			require.NoError(t, err)

			files[filepath.ToSlash(rel)] = string(content)
		}
	}

	return files
}

// TestGenerateAll_matchesStandalone verifies that generating several configs
// in one process, with the package cache shared between them, writes exactly
// the files that generating each config in its own process writes, and that
// the batch run lists and type checks the shared autobind package fewer times
// than the standalone runs do. Both sides run in child processes, so neither
// sees process-wide state left by the other tests of this binary.
//
// The go list invocations are counted with a logging go wrapper first in
// PATH (the child processes inherit it). The exact counts depend on gqlgen:
// Init lists a package once per process, and modelgen calls
// ReloadAllPackages after it writes a model file, which drops every
// non-gqlgen package from the cache. With the gqlgen version in go.mod,
// standalone lists the models package twice per config and the batch run
// once plus once per config (every config of the fixture writes a model
// file). The assertions only pin the property so that a gqlgen update that
// reloads less does not fail the test.
func TestGenerateAll_matchesStandalone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the go wrapper is a shell script")
	}

	dirs := multiConfigDirs(t)

	root, err := filepath.Abs(multiConfigRoot)
	require.NoError(t, err)

	// Standalone: every config in its own process, as separate gqlgenc
	// invocations would run.
	removeMultiConfigOutputs(t)

	standaloneCalls := generator.CountGoListCalls(t, []string{multiConfigModels}, func() {
		for _, dir := range dirs {
			generator.GenerateInFreshProcess(t, "", dir)
		}
	})

	standalone := readMultiConfigOutputs(t)
	require.Len(t, standalone, 8, "a and d write two files each, b three (one of them from c), c one")

	// Batch: one call from the fixture root with relative directories, as
	// the README example does; the cache built by a is handed to the others.
	removeMultiConfigOutputs(t)

	batchCalls := generator.CountGoListCalls(t, []string{multiConfigModels}, func() {
		generator.GenerateInFreshProcess(t, root, multiConfigNames...)
	})

	batch := readMultiConfigOutputs(t)

	require.Equal(t, standalone, batch)

	t.Logf("go list calls naming %s: standalone=%d batch=%d", multiConfigModels, standaloneCalls[multiConfigModels], batchCalls[multiConfigModels])

	require.GreaterOrEqual(t, standaloneCalls[multiConfigModels], len(dirs), "every standalone run lists the package at least once")
	require.Less(t, batchCalls[multiConfigModels], standaloneCalls[multiConfigModels], "the batch run must share the package between configs")
	require.LessOrEqual(t, batchCalls[multiConfigModels], 1+len(dirs), "batch: at most once in Init plus modelgen's ReloadAllPackages of every config")

	// The generated packages compile, including b/actual which holds the
	// output of both b and c.
	for _, name := range multiConfigNames {
		pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedTypes}, "./"+filepath.ToSlash(filepath.Join(multiConfigRoot, name, "actual")))
		require.NoError(t, err)
		require.Len(t, pkgs, 1)
		require.Empty(t, pkgs[0].Errors, name)
	}
}

// TestGenerateAll_generatesDuplicatesOnce verifies that a config given more
// than once is generated once: the batch run of a and a lists the models
// package as often as a single run of a does.
func TestGenerateAll_generatesDuplicatesOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the go wrapper is a shell script")
	}

	dirs := multiConfigDirs(t)

	removeMultiConfigOutputs(t)

	single := generator.CountGoListCalls(t, []string{multiConfigModels}, func() {
		generator.GenerateInFreshProcess(t, "", dirs[0])
	})

	removeMultiConfigOutputs(t)

	twice := generator.CountGoListCalls(t, []string{multiConfigModels}, func() {
		generator.GenerateInFreshProcess(t, "", dirs[0], dirs[0])
	})

	require.Equal(t, single[multiConfigModels], twice[multiConfigModels])
}

// TestGenerateAll_restoresWorkingDirectory verifies that GenerateAll leaves
// the working directory where it found it, even though it changes into every
// config directory on the way, also when a config fails to load after the
// change. Nothing is generated, so the process-wide state of gqlgen stays
// untouched for the other tests of this binary.
func TestGenerateAll_restoresWorkingDirectory(t *testing.T) {
	before, err := os.Getwd()
	require.NoError(t, err)

	invalid := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(invalid, "gqlgenc.yml"), []byte("client: [not a mapping]\n"), 0o600))

	err = generator.GenerateAll(context.Background(), []string{invalid})
	require.Error(t, err)

	var loadErr *generator.LoadConfigError

	require.True(t, errors.As(err, &loadErr))

	after, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// TestGenerateAll_errorNamesConfig verifies that a failure in one of several
// configs is reported with the directory the caller passed, and that a missing
// config is reported as a load error. The missing config comes first, so
// nothing is generated in this process.
func TestGenerateAll_errorNamesConfig(t *testing.T) {
	dirs := multiConfigDirs(t)
	missing := filepath.Join(t.TempDir(), "nowhere")

	err := generator.GenerateAll(context.Background(), []string{missing, dirs[0]})
	require.Error(t, err)

	var loadErr *generator.LoadConfigError

	require.True(t, errors.As(err, &loadErr))
	require.Equal(t, missing, loadErr.ConfigDir)
	require.True(t, strings.HasPrefix(err.Error(), missing+": "), err.Error())

	// A single config is reported without the prefix, as before.
	err = generator.GenerateAll(context.Background(), []string{missing})
	require.Error(t, err)
	require.True(t, errors.As(err, &loadErr))
	require.False(t, strings.HasPrefix(err.Error(), missing+": "), err.Error())
}
