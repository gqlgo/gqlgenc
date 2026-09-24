/*
Copyright (c) 2020 gqlgen authors
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:
The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package fileglob expands the file patterns accepted by the schema and query
// settings. It is a copy of the glob handling in gqlgen's config loading,
// which gqlgen does not export.
package fileglob

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var path2regex = strings.NewReplacer(
	`.`, `\.`,
	`*`, `.+`,
	`\`, `[\\/]`,
	`/`, `[\\/]`,
)

// Expand resolves file patterns into the files they match.
//
// Arguments:
//   - patterns: file paths or glob patterns; a pattern containing "**" matches any number of directories
//
// Returns:
//   - []string: the matched paths in pattern order, each path listed once
//   - error: non-nil if a pattern is malformed or a directory walk fails
//
// Preconditions:
//   - none
//
// Postconditions:
//   - a pattern that matches nothing contributes no paths and no error
func Expand(patterns []string) ([]string, error) {
	var files []string

	for _, pattern := range patterns {
		matches, err := expand(pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			if !slices.Contains(files, match) {
				files = append(files, match)
			}
		}
	}

	return files, nil
}

// expand resolves one pattern.
//
// Arguments:
//   - pattern: a file path or glob pattern
//
// Returns:
//   - []string: the matched paths
//   - error: non-nil if the pattern is malformed or a directory walk fails
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func expand(pattern string) ([]string, error) {
	// for ** we want to override default globbing patterns and walk all
	// subdirectories to match schema files.
	if !strings.Contains(pattern, "**") {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to glob schema filename %s: %w", pattern, err)
		}

		return matches, nil
	}

	pathParts := strings.SplitN(pattern, "**", 2)
	rest := strings.TrimPrefix(strings.TrimPrefix(pathParts[1], `\`), `/`)
	// turn the rest of the glob into a regex, anchored only at the end because ** allows
	// for any number of dirs in between and walk will let us match against the full path name
	globRe := regexp.MustCompile(path2regex.Replace(rest) + `$`)

	var matches []string

	err := filepath.Walk(pathParts[0], func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if globRe.MatchString(strings.TrimPrefix(path, pathParts[0])) {
			matches = append(matches, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk schema at root %s: %w", pathParts[0], err)
	}

	return matches, nil
}
