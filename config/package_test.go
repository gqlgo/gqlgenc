package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
)

// writeFiles は、root の下に files (root からの相対パス → 内容) を書く。
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("failed to create directory for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}
}

func TestNewPackageConfig(t *testing.T) {
	t.Parallel()

	type fields struct {
		// files は、テストごとの一時ディレクトリに置くファイル (相対パス → 内容)
		files map[string]string
	}

	type args struct {
		// filename は、一時ディレクトリからの相対パス。空なら空のまま渡す
		filename string
	}

	type want struct {
		// config の Filename は、一時ディレクトリからの相対パスで書く
		config PackageConfig
		err    error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "出力先のディレクトリのパッケージ名とimportパスを決める",
			fields: fields{
				files: map[string]string{
					"go.mod":           "module example.com/shop\n",
					"domain/status.go": "package domain\n",
				},
			},
			args: args{
				filename: "domain/query_gen.go",
			},
			want: want{
				config: PackageConfig{
					Filename:   "domain/query_gen.go",
					Package:    "domain",
					ImportPath: "example.com/shop/domain",
				},
			},
		},
		{
			// 初めて生成するときは、出力先のディレクトリがまだ無い
			name: "出力先のディレクトリが無ければディレクトリ名をパッケージ名にする",
			fields: fields{
				files: map[string]string{
					"go.mod": "module example.com/shop\n",
				},
			},
			args: args{
				filename: "gen/client_gen.go",
			},
			want: want{
				config: PackageConfig{
					Filename:   "gen/client_gen.go",
					Package:    "gen",
					ImportPath: "example.com/shop/gen",
				},
			},
		},
		{
			// 境界値: generate.model.file などを省略した出力
			name: "ファイル名が空なら生成しない出力を返す",
			args: args{
				filename: "",
			},
			want: want{
				config: PackageConfig{},
			},
		},
		{
			name: "Goのファイルでなければエラー",
			args: args{
				filename: "gen/query_gen.txt",
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeFiles(t, root, tt.fields.files)

			filename := tt.args.filename
			wantConfig := tt.want.config
			if filename != "" {
				filename = filepath.Join(root, filename)
			}
			if wantConfig.Filename != "" {
				wantConfig.Filename = filepath.ToSlash(filepath.Join(root, wantConfig.Filename))
			}

			got, err := newPackageConfig(filename)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(wantConfig, got); diff != "" {
				t.Errorf("config diff(-want +got): %s", diff)
			}
		})
	}
}

func TestPackageNameForDir(t *testing.T) {
	t.Parallel()

	type fields struct {
		// files は、テストごとの一時ディレクトリに置くファイル (相対パス → 内容)
		files map[string]string
	}

	type args struct {
		// dir は、一時ディレクトリからの相対パス
		dir string
	}

	type want struct {
		packageName string
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "Goのファイルのpackage句の名前を返す",
			fields: fields{
				files: map[string]string{"domain/status.go": "package model\n"},
			},
			args: args{dir: "domain"},
			want: want{packageName: "model"},
		},
		{
			name: "複数のファイルがあれば名前の順で最初のファイルの名前を返す",
			fields: fields{
				files: map[string]string{
					"domain/a.go": "package first\n",
					"domain/b.go": "package second\n",
				},
			},
			args: args{dir: "domain"},
			want: want{packageName: "first"},
		},
		{
			name: "package句を読めないファイルは飛ばす",
			fields: fields{
				files: map[string]string{
					"domain/a.go": "これは Go のソースではない\n",
					"domain/b.go": "package second\n",
				},
			},
			args: args{dir: "domain"},
			want: want{packageName: "second"},
		},
		{
			name: "Goでないファイルは読まない",
			fields: fields{
				files: map[string]string{"domain/README.md": "package readme\n"},
			},
			args: args{dir: "domain"},
			want: want{packageName: "domain"},
		},
		{
			// 境界値: 初めて生成するときは、出力先のディレクトリがまだ無い
			name: "ディレクトリが無ければディレクトリ名を返す",
			args: args{dir: "gen"},
			want: want{packageName: "gen"},
		},
		{
			name: "ディレクトリ名のパッケージ名に使えない文字は_にする",
			fields: fields{
				files: map[string]string{"my-app.v2/README.md": ""},
			},
			args: args{dir: "my-app.v2"},
			want: want{packageName: "my_app_v2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeFiles(t, root, tt.fields.files)
			dir := filepath.Join(root, tt.args.dir)

			got := packageNameForDir(dir)

			if diff := cmp.Diff(tt.want.packageName, got); diff != "" {
				t.Errorf("packageName diff(-want +got): %s", diff)
			}

			// model は gqlgen の modelgen が生成するため、gqlgen と同じ名前にならなければならない
			gqlgenOutput := gqlgenconfig.PackageConfig{Filename: filepath.Join(dir, "model_gen.go")}
			if err := gqlgenOutput.Check(); err != nil {
				t.Fatalf("gqlgen の PackageConfig.Check: %v", err)
			}
			if diff := cmp.Diff(gqlgenOutput.Package, got); diff != "" {
				t.Errorf("gqlgen のパッケージ名との diff(-want +got): %s", diff)
			}
		})
	}
}

// gqlgen の import パスの解決は、排他していないパッケージ変数のキャッシュに書くため、gqlgen と比べる
// このテストは並列に実行しない。
func TestImportPathForDir(t *testing.T) {
	type fields struct {
		// files は、テストごとの一時ディレクトリに置くファイル (相対パス → 内容)
		files map[string]string
	}

	type args struct {
		// dir は、一時ディレクトリからの相対パス
		dir string
	}

	type want struct {
		importPath string
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "go.modのあるディレクトリはmoduleパスを返す",
			fields: fields{
				files: map[string]string{"go.mod": "module example.com/shop\n"},
			},
			args: args{dir: "."},
			want: want{importPath: "example.com/shop"},
		},
		{
			name: "go.modより下のディレクトリはmoduleパスに相対パスを足す",
			fields: fields{
				files: map[string]string{
					"go.mod":                 "module example.com/shop\n",
					"domain/model/status.go": "package model\n",
				},
			},
			args: args{dir: "domain/model"},
			want: want{importPath: "example.com/shop/domain/model"},
		},
		{
			// 初めて生成するときは、出力先のディレクトリがまだ無い
			name: "まだ無いディレクトリも親のgo.modから決める",
			fields: fields{
				files: map[string]string{"go.mod": "module example.com/shop\n"},
			},
			args: args{dir: "gen"},
			want: want{importPath: "example.com/shop/gen"},
		},
		{
			name: "go.modが入れ子のときは近いgo.modから決める",
			fields: fields{
				files: map[string]string{
					"go.mod":       "module example.com/shop\n",
					"tools/go.mod": "module example.com/tools\n",
				},
			},
			args: args{dir: "tools/cmd"},
			want: want{importPath: "example.com/tools/cmd"},
		},
		{
			name: "go.modの先頭のコメントは読み飛ばす",
			fields: fields{
				files: map[string]string{"go.mod": "// module example.com/comment\n\nmodule example.com/shop\n\ngo 1.27\n"},
			},
			args: args{dir: "domain"},
			want: want{importPath: "example.com/shop/domain"},
		},
		{
			// 境界値: 一時ディレクトリは、どの module にも GOPATH にも入っていない
			name: "go.modもGOPATHも無ければ空文字を返す",
			args: args{dir: "gen"},
			want: want{importPath: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// gqlgen の import パスの解決が並列に呼べないため、t.Parallel() を呼ばない
			root := t.TempDir()
			writeFiles(t, root, tt.fields.files)
			dir := filepath.Join(root, tt.args.dir)

			got := importPathForDir(dir)

			if diff := cmp.Diff(tt.want.importPath, got); diff != "" {
				t.Errorf("importPath diff(-want +got): %s", diff)
			}

			// model は gqlgen の modelgen が生成するため、gqlgen と同じ import パスにならなければならない
			gqlgenOutput := gqlgenconfig.PackageConfig{Filename: filepath.Join(dir, "model_gen.go")}
			if diff := cmp.Diff(gqlgenOutput.ImportPath(), got); diff != "" {
				t.Errorf("gqlgen の import パスとの diff(-want +got): %s", diff)
			}
		})
	}
}
