package generator_test

import (
	"context"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

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
	"multi_config": true,
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
		// directory must be a golden fixture, so a misnamed one fails loudly
		if nonGoldenFixtures[dir.Name()] {
			continue
		}

		testDirs = append(testDirs, dir.Name())
	}

	return testDirs
}
