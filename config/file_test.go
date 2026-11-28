package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSchemaFilenames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// ** が複数あるグロブにマッチするファイル。2 つ目の ** が bb/deep の複数階層をまたぐ
	matchPath := filepath.Join(root, "aa", "mid", "bb", "deep", "schema.graphql")
	// mid 階層を含まないためマッチしないファイル
	noMatchPath := filepath.Join(root, "aa", "other", "schema.graphql")
	for _, p := range []string{matchPath, noMatchPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte("type Query { _: Boolean }"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	type args struct {
		schemaFilenameGlobs []string
	}

	type want struct {
		filenames []string
		err       error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// パス中に ** が複数あっても、最初の ** で walk の起点を切り、残りの **
			// はパターン (* -> .+) として扱われ、複数階層をまたいで一致する
			name: "**がパス中に複数あっても最初の**で起点を切り残りはパターンとして一致する",
			args: args{
				schemaFilenameGlobs: []string{filepath.Join(root, "**", "mid", "**", "schema.graphql")},
			},
			want: want{
				filenames: []string{matchPath},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := schemaFilenames(tt.args.schemaFilenameGlobs)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.filenames, got, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("filenames diff(-want +got): %s", diff)
			}
		})
	}
}
