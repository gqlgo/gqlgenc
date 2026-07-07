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

// Package glob expands file path glob patterns, with support for "**" to match
// any number of intermediate directories.
package glob

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var path2regex = strings.NewReplacer(
	`.`, `\.`,
	`*`, `.+`,
	`\`, `[\\/]`,
	`/`, `[\\/]`,
)

// Files expands the given glob patterns into a deduplicated list of matching
// file paths, preserving first-seen order. A "**" in a pattern overrides the
// default globbing and walks all subdirectories to match the remaining pattern.
func Files(patterns []string) ([]string, error) {
	seen := make(map[string]struct{})

	var files []string

	for _, pattern := range patterns {
		matches, err := expand(pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}

			seen[match] = struct{}{}
			files = append(files, match)
		}
	}

	return files, nil
}

func expand(pattern string) ([]string, error) {
	before, after, found := strings.Cut(pattern, "**")
	if !found {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", pattern, err)
		}

		return matches, nil
	}

	rest := strings.TrimPrefix(strings.TrimPrefix(after, `\`), `/`)
	// anchor the regex only at the end because ** allows for any number of dirs
	// in between and walk will let us match against the full path name
	globRe := regexp.MustCompile(path2regex.Replace(rest) + `$`)

	var matches []string

	if err := filepath.Walk(before, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if globRe.MatchString(strings.TrimPrefix(path, before)) {
			matches = append(matches, path)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk %q: %w", before, err)
	}

	return matches, nil
}
