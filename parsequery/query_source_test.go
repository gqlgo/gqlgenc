package parsequery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestLoadQuerySources(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.graphql"), "query A { a }")
	writeFile(t, filepath.Join(dir, "b.graphql"), "query B { b }")
	writeFile(t, filepath.Join(dir, "nested", "deep", "c.graphql"), "query C { c }")
	writeFile(t, filepath.Join(dir, "nested", "ignored.txt"), "not graphql")

	tests := []struct {
		name      string
		patterns  []string
		wantNames []string
		wantErr   string
	}{
		{
			name:      "explicit file",
			patterns:  []string{filepath.Join(dir, "a.graphql")},
			wantNames: []string{"a.graphql"},
		},
		{
			name:      "glob in one directory",
			patterns:  []string{filepath.Join(dir, "*.graphql")},
			wantNames: []string{"a.graphql", "b.graphql"},
		},
		{
			name:      "double star walks subdirectories",
			patterns:  []string{filepath.Join(dir, "**", "*.graphql")},
			wantNames: []string{"a.graphql", "b.graphql", "nested/deep/c.graphql"},
		},
		{
			name:      "the same file matched twice is loaded once",
			patterns:  []string{filepath.Join(dir, "a.graphql"), filepath.Join(dir, "*.graphql")},
			wantNames: []string{"a.graphql", "b.graphql"},
		},
		{
			name:      "no match yields no sources",
			patterns:  []string{filepath.Join(dir, "missing", "*.graphql")},
			wantNames: []string{},
		},
		{
			// filepath.Glob reports no match for a missing explicit path, so it is not an error today.
			name:      "explicit missing file yields no sources",
			patterns:  []string{filepath.Join(dir, "missing.graphql")},
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sources, err := LoadQuerySources(tt.patterns)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			names := make([]string, 0, len(sources))
			for _, source := range sources {
				rel, err := filepath.Rel(dir, filepath.FromSlash(source.Name))
				require.NoError(t, err)

				names = append(names, filepath.ToSlash(rel))
				require.NotEmpty(t, source.Input)
			}

			require.Equal(t, tt.wantNames, names)
		})
	}
}
