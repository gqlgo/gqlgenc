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
	"slices"
	"strings"
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

	// 最初の ** の前を walk の起点にし、起点からの相対パスを ** 以降のパターンとパスの要素ごとに
	// 照合する。** の要素は 0 個以上のディレクトリに一致し、それ以外の要素は filepath.Match の
	// 規則 (* は区切りを含まない 0 文字以上、? は 1 文字) で 1 つの要素に一致する。
	// 正規表現へ置き換えると、+ ( ? などのファイル名の文字が正規表現の記号になり、* が要素の
	// 先頭の部分 (superuser の super) にも一致する。
	root := before
	if root == "" {
		root = "."
	}

	patternSegments := append([]string{"**"}, splitSegments(after)...)
	for _, segment := range patternSegments {
		// filepath.Match は、照合する名前によらず不正なパターンに ErrBadPattern を返す。
		if _, err := filepath.Match(segment, ""); err != nil {
			return nil, fmt.Errorf("glob %q: %w", pattern, err)
		}
	}

	var matches []string

	if err := filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path of %q: %w", path, err)
		}

		if matchSegments(patternSegments, splitSegments(rel)) {
			matches = append(matches, path)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk %q: %w", root, err)
	}

	return matches, nil
}

// splitSegments はパスを要素に分ける。区切りは / と OS の区切り文字で、空の要素と "." は除く。
func splitSegments(path string) []string {
	segments := strings.FieldsFunc(filepath.ToSlash(path), func(r rune) bool { return r == '/' })

	return slices.DeleteFunc(segments, func(segment string) bool { return segment == "." })
}

// matchSegments は、パスの要素 name がパターンの要素 pattern に一致するかを返す。pattern の "**" は
// 0 個以上の要素に一致し、それ以外の要素は filepath.Match で name の 1 つの要素と照合する。
//
// 事前条件: pattern の要素は filepath.Match の正しいパターンである。
func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}

	if pattern[0] == "**" {
		for i := 0; i <= len(name); i++ {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}

	if len(name) == 0 {
		return false
	}

	ok, _ := filepath.Match(pattern[0], name[0])

	return ok && matchSegments(pattern[1:], name[1:])
}
