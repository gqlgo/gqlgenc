package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Yamashou/gqlgenc/v3/internal/glob"

	"github.com/vektah/gqlparser/v2/ast"
)

// FindConfigFile walks up from path looking for the closest match.
func FindConfigFile(path string, cfgFilenames []string) (string, error) {
	var err error

	var dir string
	if path == "." {
		dir, err = os.Getwd()
	} else {
		dir = path
		_, err = os.Stat(dir)
	}

	if err != nil {
		return "", fmt.Errorf("unable to get directory \"%s\" to findCfg: %w", dir, err)
	}

	startDir := dir
	cfg := findConfigInDir(dir, cfgFilenames)

	for cfg == "" && dir != filepath.Dir(dir) {
		dir = filepath.Dir(dir)
		cfg = findConfigInDir(dir, cfgFilenames)
	}

	if cfg == "" {
		return "", fmt.Errorf("config file not found in %q or any parent directory (looked for %s)", startDir, strings.Join(cfgFilenames, ", "))
	}

	return cfg, nil
}

func findConfigInDir(dir string, cfgFilenames []string) string {
	for _, cfgName := range cfgFilenames {
		path := filepath.Join(dir, cfgName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func schemaFilenames(schemaFilenameGlobs []string) ([]string, error) {
	filenames, err := glob.Files(schemaFilenameGlobs)
	if err != nil {
		return nil, fmt.Errorf("schema files: %w", err)
	}

	slices.Sort(filenames)

	return filenames, nil
}

func schemaFileSources(schemaFilenames []string) ([]*ast.Source, error) {
	sources := make([]*ast.Source, 0, len(schemaFilenames))

	for _, schemaFilename := range schemaFilenames {
		schemaFilename = filepath.ToSlash(schemaFilename)

		schemaRaw, err := os.ReadFile(schemaFilename)
		if err != nil {
			return nil, fmt.Errorf("unable to open schema: %w", err)
		}

		sources = append(sources, &ast.Source{Name: schemaFilename, Input: string(schemaRaw)})
	}

	return sources, nil
}
