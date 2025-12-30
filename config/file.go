package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// looking for the closest match.
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

	cfg := findConfigInDir(dir, cfgFilenames)

	for cfg == "" && dir != filepath.Dir(dir) {
		dir = filepath.Dir(dir)
		cfg = findConfigInDir(dir, cfgFilenames)
	}

	if cfg == "" {
		return "", fmt.Errorf("not found Config. Config could not be found. Please make sure the name of the file is correct. want={.gqlgenc.yml, gqlgenc.yml, gqlgenc.yaml}, got=%s: %w", dir, err)
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
	path2regex := strings.NewReplacer(
		`.`, `\.`,
		`*`, `.+`,
		`\`, `[\\/]`,
		`/`, `[\\/]`,
	)

	allSchemaFilenames := make(map[string]struct{})

	for _, schemaFilenameGlob := range schemaFilenameGlobs {
		var schemaFilenames []string

		if strings.Contains(schemaFilenameGlob, "**") {
			// for ** we want to override default globbing patterns and walk all
			// subdirectories to match schema files.
			pathParts := strings.SplitN(schemaFilenameGlob, "**", 2)
			rest := strings.TrimPrefix(strings.TrimPrefix(pathParts[1], `\`), `/`)
			// turn the rest of the glob into a regex, anchored only at the end because ** allows
			// for any number of dirs in between and walk will let us match against the full path name
			globRe := regexp.MustCompile(path2regex.Replace(rest) + `$`)

			if err := filepath.Walk(pathParts[0], func(path string, _ os.FileInfo, err error) error {
				if err != nil {
					return fmt.Errorf("%w", err)
				}

				if globRe.MatchString(strings.TrimPrefix(path, pathParts[0])) {
					schemaFilenames = append(schemaFilenames, path)
				}

				return nil
			}); err != nil {
				return nil, fmt.Errorf("failed to walk schema at root %s: %w", pathParts[0], err)
			}
		} else {
			var err error

			schemaFilenames, err = filepath.Glob(schemaFilenameGlob)
			if err != nil {
				return nil, fmt.Errorf("failed to glob schema filename %s: %w", schemaFilenameGlob, err)
			}
		}

		for _, schemaFilename := range schemaFilenames {
			allSchemaFilenames[schemaFilename] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(allSchemaFilenames)), nil
}

func schemaFileSources(schemaFilenames []string) ([]*ast.Source, error) {
	sources := make([]*ast.Source, 0, len(schemaFilenames))

	for _, schemaFilename := range schemaFilenames {
		schemaFilename = filepath.ToSlash(schemaFilename)

		var err error

		var schemaRaw []byte

		schemaRaw, err = os.ReadFile(schemaFilename)
		if err != nil {
			return nil, fmt.Errorf("unable to open schema: %w", err)
		}

		sources = append(sources, &ast.Source{Name: schemaFilename, Input: string(schemaRaw)})
	}

	return sources, nil
}
