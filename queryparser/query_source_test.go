package queryparser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestLoadQuerySources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// ** が複数あるグロブにマッチするファイル。2 つ目の ** が bb/deep の複数階層をまたぐ
	matchPath := filepath.Join(root, "aa", "mid", "bb", "deep", "query.graphql")
	// mid 階層を含まないためマッチしないファイル
	noMatchPath := filepath.Join(root, "aa", "other", "query.graphql")
	for _, p := range []string{matchPath, noMatchPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte("query Q { __typename }"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	type args struct {
		queryFileNames []string
	}

	type want struct {
		names []string
		err   error
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
				queryFileNames: []string{filepath.Join(root, "**", "mid", "**", "query.graphql")},
			},
			want: want{
				names: []string{filepath.ToSlash(matchPath)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := LoadQuerySources(tt.args.queryFileNames)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			gotNames := make([]string, 0, len(got))
			for _, src := range got {
				gotNames = append(gotNames, src.Name)
			}
			if diff := cmp.Diff(tt.want.names, gotNames, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("names diff(-want +got): %s", diff)
			}
		})
	}
}
