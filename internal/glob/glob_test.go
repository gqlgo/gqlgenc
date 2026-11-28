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
	// ** の後ろのパターンの照合に使うファイル。起点の直下 (** が 0 個のディレクトリに一致する) と、
	// 正規表現の記号になる文字 (+ ( ) を含む名前を置く
	userPath := filepath.Join(root, "users", "user.graphql")
	userAdminPath := filepath.Join(root, "users", "admin", "user_admin.graphql")
	superuserPath := filepath.Join(root, "users", "superuser.graphql")
	groupPath := filepath.Join(root, "routes", "(group).graphql")
	pagePath := filepath.Join(root, "routes", "blog", "+page.graphql")
	singleCharPath := filepath.Join(root, "routes", "a.graphql")
	for _, p := range []string{matchPath, noMatchPath, userPath, userAdminPath, superuserPath, groupPath, pagePath, singleCharPath} {
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
			// は 0 個以上のディレクトリとして、複数階層をまたいで一致する
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
			// ** の後ろの * は要素の中の 0 文字以上に一致する。user*.graphql は user.graphql にも
			// 一致し、要素の先頭が user でない superuser.graphql には一致しない
			name: "**の後ろの*は0文字にも一致し要素の途中からは一致しない",
			args: args{
				patterns: []string{filepath.Join(root, "users", "**", "user*.graphql")},
			},
			want: want{
				files: []string{userPath, userAdminPath},
			},
		},
		{
			// ** の後ろのファイル名は要素全体で照合し、superuser.graphql には一致しない
			name: "**の後ろのファイル名は要素全体で照合する",
			args: args{
				patterns: []string{filepath.Join(root, "users", "**", "user.graphql")},
			},
			want: want{
				files: []string{userPath},
			},
		},
		{
			// 正規表現の記号になる + ( ) を含むファイル名にも、そのまま一致する
			name: "**の後ろのファイル名の+と括弧はその文字に一致する",
			args: args{
				patterns: []string{
					filepath.Join(root, "routes", "**", "(group).graphql"),
					filepath.Join(root, "routes", "**", "+page.graphql"),
				},
			},
			want: want{
				files: []string{groupPath, pagePath},
			},
		},
		{
			// ** の後ろの ? は 1 文字に一致する
			name: "**の後ろの?は1文字に一致する",
			args: args{
				patterns: []string{filepath.Join(root, "routes", "**", "?.graphql")},
			},
			want: want{
				files: []string{singleCharPath},
			},
		},
		{
			// ** の後ろの不正なパターン (閉じていない [) はエラーにする
			name: "**の後ろの不正なパターンはエラー",
			args: args{
				patterns: []string{filepath.Join(root, "**", "[.graphql")},
			},
			want: want{
				err: filepath.ErrBadPattern,
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
