package generator_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gqlgo/gqlgenc/config"
	"github.com/gqlgo/gqlgenc/generator"
	"github.com/stretchr/testify/suite"
	"golang.org/x/tools/go/packages"
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

func (s *Suite) TestGenerator_withTestData() {
	dirs := s.getTestDirs()

	for _, dir := range dirs {
		s.Run(dir, func() {
			// temporary change working directory
			s.useDirForTest(filepath.Join("testdata", dir))

			// load config
			cfg, err := config.LoadConfig("./gqlgenc.yml")
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
			s.Empty(pkgs[0].Errors)

			// reset expected files if there are no expected files,
			// allowing reset of expected files by removing them
			if len(expectedFiles) == 0 {
				for path, content := range actualFiles {
					s.T().Logf("resetting expected file %s", path)
					expectedPath := filepath.Join(expected, path)
					_ = os.MkdirAll(filepath.Dir(expectedPath), 0o700)
					err = os.WriteFile(expectedPath, []byte(content), 0o644)
					s.Require().NoError(err)
				}
				return
			}

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

// useDir changes the current working directory to the given directory
// and returns a function that can be used to restore the original
// working directory.
func (s *Suite) useDirForTest(dir string) {
	wd, err := os.Getwd()
	s.Require().NoError(err)
	err = os.Chdir(dir)
	s.Require().NoError(err)

	// cleanup actual directory
	_ = os.RemoveAll(filepath.Join(dir, actual))

	s.T().Cleanup(func() {
		s.Require().NoError(os.Chdir(wd))
	})
}

func (s *Suite) loadFiles(dir string) map[string]string {
	files := make(map[string]string)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return files
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
		testDirs = append(testDirs, dir.Name())
	}
	return testDirs
}
