package generator_test

import (
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"golang.org/x/tools/go/packages"

	"github.com/gqlgo/gqlgenc/config"
	"github.com/gqlgo/gqlgenc/generator"
)

type Suite struct {
	suite.Suite
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

const (
	expected = "expected"
	actual   = "actual"
)

var update = flag.Bool("update", false, "rewrite the expected files with the generated output")

// nonGoldenFixtures are the testdata directories used by other tests, which
// TestGenerator_withTestData skips.
var nonGoldenFixtures = map[string]bool{
	"multi_config":             true,
	"field_name_collision":     true, // expects an error; see TestGenerator_fieldNameCollision
	"client_only_unbound_type": true, // expects an error; see TestGenerator_clientOnlyUnboundType
	"client_only_nested_input": true, // see TestGenerator_clientOnlyNestedInputPartlyBound
}

func (s *Suite) TestGenerator_withTestData() {
	dirs := s.getTestDirs()

	for _, dir := range dirs {
		s.Run(dir, func() {
			// temporary change working directory
			s.useDirForTest(filepath.Join("testdata", dir))

			// load config, whichever of the accepted file names the fixture uses;
			// the file must be in the fixture itself, not in a parent directory
			cfgFile, err := config.FindConfigFile(".")
			s.Require().NoError(err)

			cwd, err := os.Getwd()
			s.Require().NoError(err)
			s.Require().Equal(cwd, filepath.Dir(cfgFile), "fixture has no config of its own")

			cfg, err := config.LoadConfig(cfgFile)
			s.Require().NoError(err)

			// disable unnecessary validations
			cfg.GQLConfig.SkipValidation = true
			cfg.GQLConfig.SkipModTidy = true

			// generate code
			err = generator.Generate(context.Background(), cfg)
			s.Require().NoError(err)

			// load all files
			expectedFiles := s.loadFiles(expected)
			actualFiles := s.loadFiles(actual)

			s.Require().NotEmpty(actualFiles, "no actual files found")

			// verify that generated code compiles
			pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedTypes}, "./"+actual)
			s.Require().NoError(err)
			s.Require().Len(pkgs, 1)
			// コンパイルできない出力は -update でも expected に書き込まない
			s.Require().Empty(pkgs[0].Errors)

			// rewrite the expected files from the generated output when -update is given
			if *update {
				// expected を actual の写しにする（生成されなくなったファイルも削除する）
				s.Require().NoError(os.RemoveAll(expected))

				for path, content := range actualFiles {
					s.T().Logf("updating expected file %s", path)
					expectedPath := filepath.Join(expected, path)
					_ = os.MkdirAll(filepath.Dir(expectedPath), 0o700)
					err = os.WriteFile(expectedPath, []byte(content), 0o644)
					s.Require().NoError(err)
				}

				return
			}

			s.Require().NotEmpty(expectedFiles, "no expected files found; run with -update to create them")

			// ファイルの過不足を検出する
			s.ElementsMatch(slices.Collect(maps.Keys(expectedFiles)), slices.Collect(maps.Keys(actualFiles)),
				"generated files differ from expected files; run with -update to rewrite them")

			// compare expected and actual files
			for path, expectedContent := range expectedFiles {
				actualContent, ok := actualFiles[path]
				if s.True(ok, "expected file %s not found", path) {
					s.Equal(expectedContent, actualContent)
				}
			}
		})
	}
}

// TestGenerator_nilGenerateConfig verifies that Generate tolerates a Config
// whose Generate section was left unset by a caller that built it by hand.
func (s *Suite) TestGenerator_nilGenerateConfig() {
	s.useDirForTest(filepath.Join("testdata", "multiple_queries"))

	cfg, err := config.LoadConfig("./gqlgenc.yml")
	s.Require().NoError(err)

	cfg.GQLConfig.SkipValidation = true
	cfg.GQLConfig.SkipModTidy = true
	cfg.Generate = nil

	s.Require().NotPanics(func() {
		err = generator.Generate(context.Background(), cfg)
	})
	s.Require().NoError(err)
}

// TestGenerator_clientOnlyUnboundType verifies that a client-only config reports
// an unbound type instead of panicking, and that a first run with no previous
// output leaves neither the client file nor a backup behind.
func (s *Suite) TestGenerator_clientOnlyUnboundType() {
	s.useDirForTest(filepath.Join("testdata", "client_only_unbound_type"))

	cfg, err := config.LoadConfig("./gqlgenc.yml")
	s.Require().NoError(err)

	cfg.GQLConfig.SkipValidation = true
	cfg.GQLConfig.SkipModTidy = true

	s.Require().NotPanics(func() {
		err = generator.Generate(context.Background(), cfg)
	})
	s.Require().Error(err)
	s.Require().ErrorContains(err, "ExtraFilter")
	s.Require().ErrorContains(err, "GetExtra")
	s.Require().ErrorContains(err, "autobind")
	s.Require().ErrorContains(err, "model.filename")

	_, statErr := os.Stat(filepath.Join("actual", "client_gen.go"))
	s.Require().ErrorIs(statErr, os.ErrNotExist)
	s.Require().NoError(assertNoOutputBackup("."))
}

// TestGenerator_preservesOutput verifies that generating unchanged sources
// again keeps the previous bytes and mtimes, and that a later query error
// puts those files back without leaving a backup behind.
func (s *Suite) TestGenerator_preservesOutput() {
	dir := s.copyFixture("multiple_queries")

	// Each run loads its own config; see loadConfig.
	cfg := s.loadConfig()
	s.Require().NoError(generator.Generate(context.Background(), cfg))

	clientPath := cfg.Client.Filename
	modelPath := cfg.Model.Filename
	clientBefore := s.readFile(clientPath)
	modelBefore := s.readFile(modelPath)

	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	s.Require().NoError(os.Chtimes(clientPath, past, past))
	s.Require().NoError(os.Chtimes(modelPath, past, past))

	clientMtime := s.modTime(clientPath)
	modelMtime := s.modTime(modelPath)

	s.Require().NoError(generator.Generate(context.Background(), s.loadConfig()))

	s.Equal(clientBefore, s.readFile(clientPath))
	s.Equal(modelBefore, s.readFile(modelPath))
	s.True(clientMtime.Equal(s.modTime(clientPath)), "client mtime changed")
	s.True(modelMtime.Equal(s.modTime(modelPath)), "model mtime changed")
	s.Require().NoError(assertNoOutputBackup(dir))

	s.Require().NoError(os.WriteFile(filepath.Join("queries", "user.graphql"), []byte("query GetUser {\n"), 0o644))

	err := generator.Generate(context.Background(), s.loadConfig())
	s.Require().Error(err)
	s.Require().ErrorContains(err, "user.graphql")

	s.Equal(clientBefore, s.readFile(clientPath))
	s.Equal(modelBefore, s.readFile(modelPath))
	s.Require().NoError(assertNoOutputBackup(dir))
}

// TestGenerator_clientOnlyNestedInputPartlyBound verifies that a client-only
// config generates when `models:` binds a variable's input type but not the
// input types nested inside it, which the client never looks up.
func (s *Suite) TestGenerator_clientOnlyNestedInputPartlyBound() {
	s.useDirForTest(filepath.Join("testdata", "client_only_nested_input"))

	cfg, err := config.LoadConfig("./gqlgenc.yml")
	s.Require().NoError(err)

	cfg.GQLConfig.SkipValidation = true
	cfg.GQLConfig.SkipModTidy = true

	s.Require().NoError(generator.Generate(context.Background(), cfg))
	s.Contains(s.readFile(filepath.Join("actual", "client_gen.go")), "ListItems")
	s.Require().NoError(assertNoOutputBackup("."))
}

// TestGenerator_emptyModelsRemovesPreviousOutput verifies that a models_gen.go
// left by an earlier run is removed, with its backup, when onlyUsedModels
// leaves no model to generate.
func (s *Suite) TestGenerator_emptyModelsRemovesPreviousOutput() {
	dir := s.copyFixture("only_used_models_empty")

	cfg := s.loadConfig()
	modelPath := cfg.Model.Filename
	s.Require().NoError(os.MkdirAll(filepath.Dir(modelPath), 0o755))
	s.Require().NoError(os.WriteFile(modelPath, []byte("package generated\n\ntype Unused struct {\n\tX string\n}\n"), 0o644))

	s.Require().NoError(generator.Generate(context.Background(), cfg))

	_, err := os.Stat(modelPath)
	s.Require().ErrorIs(err, os.ErrNotExist)
	s.FileExists(cfg.Client.Filename)
	s.Require().NoError(assertNoOutputBackup(dir))
}

// TestGenerator_restoresLeftoverBackup verifies that when an interrupted run
// left only the .gqlgenc.bak of an output, a failing run puts it back, and a
// later successful run leaves no backup.
func (s *Suite) TestGenerator_restoresLeftoverBackup() {
	dir := s.copyFixture("multiple_queries")

	cfg := s.loadConfig()
	s.Require().NoError(generator.Generate(context.Background(), cfg))

	clientPath := cfg.Client.Filename
	bakPath := clientPath + ".gqlgenc.bak"
	clientBefore := s.readFile(clientPath)

	queryPath := filepath.Join("queries", "user.graphql")
	queryBefore := s.readFile(queryPath)

	// The state an interrupted run leaves: no output, only its backup.
	s.Require().NoError(os.Rename(clientPath, bakPath))
	s.Require().NoError(os.WriteFile(queryPath, []byte("query GetUser {\n"), 0o644))

	err := generator.Generate(context.Background(), s.loadConfig())
	s.Require().Error(err)
	s.Require().ErrorContains(err, "user.graphql")

	s.Equal(clientBefore, s.readFile(clientPath))
	s.Require().NoError(assertNoOutputBackup(dir))

	s.Require().NoError(os.Rename(clientPath, bakPath))
	s.Require().NoError(os.WriteFile(queryPath, []byte(queryBefore), 0o644))

	s.Require().NoError(generator.Generate(context.Background(), s.loadConfig()))

	s.Equal(clientBefore, s.readFile(clientPath))
	s.Require().NoError(assertNoOutputBackup(dir))
}

// TestGenerator_fieldNameCollision verifies that two response keys that map to the
// same Go identifier are reported as an error instead of panicking (#108).
func (s *Suite) TestGenerator_fieldNameCollision() {
	s.useDirForTest(filepath.Join("testdata", "field_name_collision"))

	cfg, err := config.LoadConfig("./gqlgenc.yml")
	s.Require().NoError(err)

	cfg.GQLConfig.SkipValidation = true
	cfg.GQLConfig.SkipModTidy = true

	s.Require().NotPanics(func() {
		err = generator.Generate(context.Background(), cfg)
	})
	s.Require().ErrorContains(err, "foo_bar")
	s.Require().ErrorContains(err, "fooBar")
	s.Require().ErrorContains(err, "FooBar")
}

// copyFixture copies testdata/name into a throwaway directory under testdata,
// changes into it for the rest of the test, and returns its absolute path.
// A system temp dir is outside this module, so gqlgen could not resolve the
// packages it binds; the go tool ignores testdata.
func (s *Suite) copyFixture(name string) string {
	s.T().Helper()

	dir, err := os.MkdirTemp(filepath.Join("testdata"), "preserve_output_")
	s.Require().NoError(err)
	dir, err = filepath.Abs(dir)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = os.RemoveAll(dir) })

	s.Require().NoError(copyDir(filepath.Join("testdata", name), dir))
	s.T().Chdir(dir)

	return dir
}

// loadConfig loads ./gqlgenc.yml as a separate gqlgenc invocation does.
// Reusing one Config keeps the operation names the client plugin records in
// Models, and the next run reports them as duplicates.
func (s *Suite) loadConfig() *config.Config {
	s.T().Helper()

	cfg, err := config.LoadConfig("./gqlgenc.yml")
	s.Require().NoError(err)

	cfg.GQLConfig.SkipValidation = true
	cfg.GQLConfig.SkipModTidy = true

	return cfg
}

func (s *Suite) readFile(path string) string {
	s.T().Helper()

	content, err := os.ReadFile(path)
	s.Require().NoError(err)

	return string(content)
}

func (s *Suite) modTime(path string) time.Time {
	s.T().Helper()

	info, err := os.Stat(path)
	s.Require().NoError(err)

	return info.ModTime()
}

// copyDir copies src into dst, skipping generated actual directories.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if info.IsDir() && info.Name() == actual && path != src {
			return filepath.SkipDir
		}

		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}

	defer in.Close()

	err = os.MkdirAll(filepath.Dir(dst), 0o755)
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}

	return closeErr
}

// assertNoOutputBackup returns an error when any .gqlgenc.bak remains under root.
func assertNoOutputBackup(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".gqlgenc.bak") {
			return fmt.Errorf("leftover backup %s", path)
		}

		return nil
	})
}

// useDirForTest removes the actual output of a previous run and changes the
// working directory to dir for the rest of the test; t.Chdir restores it.
func (s *Suite) useDirForTest(dir string) {
	s.Require().NoError(os.RemoveAll(filepath.Join(dir, actual)))
	s.T().Chdir(dir)
}

func (s *Suite) loadFiles(dir string) map[string]string {
	files := make(map[string]string)
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return files
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		files[rel] = string(content)

		return nil
	})
	s.Require().NoError(err)

	return files
}

func (s *Suite) getTestDirs() []string {
	dirs, err := os.ReadDir("testdata")
	s.Require().NoError(err)

	var testDirs []string

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		// fixtures of other tests have no config at their root; every other
		// directory must be a golden fixture, so a misnamed one fails loudly.
		// preserve_output_* is the throwaway copy from TestGenerator_preservesOutput.
		if nonGoldenFixtures[dir.Name()] || strings.HasPrefix(dir.Name(), "preserve_output_") {
			continue
		}

		testDirs = append(testDirs, dir.Name())
	}

	return testDirs
}
