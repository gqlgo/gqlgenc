package glob

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestFiles(t *testing.T) {
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
		patterns []string
	}

	type want struct {
		files []string
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
				patterns: []string{filepath.Join(root, "**", "mid", "**", "schema.graphql")},
			},
			want: want{
				files: []string{matchPath},
			},
		},
		{
			// 複数パターンが同じファイルにマッチしても、重複は1つに排除される
			name: "複数パターンの結果を重複排除する",
			args: args{
				patterns: []string{
					filepath.Join(root, "**", "schema.graphql"),
					filepath.Join(root, "aa", "mid", "bb", "deep", "schema.graphql"),
				},
			},
			want: want{
				files: []string{matchPath, noMatchPath},
			},
		},
		{
			// ** を含まないパターンは filepath.Glob で展開される
			name: "**を含まないパターンはGlobで展開する",
			args: args{
				patterns: []string{filepath.Join(root, "aa", "other", "*.graphql")},
			},
			want: want{
				files: []string{noMatchPath},
			},
		},
		{
			// どのファイルにもマッチしない場合は空を返す
			name: "マッチするファイルがない場合は空を返す",
			args: args{
				patterns: []string{filepath.Join(root, "**", "nonexistent.graphql")},
			},
			want: want{
				files: nil,
			},
		},
		{
			// 存在しないディレクトリを起点に walk するとエラーになる
			name: "存在しないディレクトリを起点にするとエラー",
			args: args{
				patterns: []string{filepath.Join("not_walkable", "**", "schema.graphql")},
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Files(tt.args.patterns)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.files, got, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("files diff(-want +got): %s", diff)
			}
		})
	}
}
