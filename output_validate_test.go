package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/internal/cmputil"
	"github.com/Yamashou/gqlgenc/v3/internal/typebind"
)

// unloadablePackage は、ビルド制約ですべてのファイルが外れるため読み込みエラーになる束縛先。
const unloadablePackage = "github.com/Yamashou/gqlgenc/v3/testdata/integration/bind-unloadable-package/domain"

// maskTestDirs は、err のメッセージにある出力先の一時ディレクトリを <dir> に、リポジトリの
// ルートを <repo> に置き換えたエラーを返す。実行する環境で変わるパスを期待値から外すために使う。
func maskTestDirs(t *testing.T, err error, dir string) error {
	t.Helper()

	if err == nil {
		return nil
	}

	repo, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("failed to get working directory: %v", wdErr)
	}

	return errors.New(strings.NewReplacer(dir, "<dir>", repo, "<repo>").Replace(err.Error()))
}

func TestValidateOutputs(t *testing.T) {
	t.Parallel()

	type args struct {
		// files は、生成が終わった時点で出力先のディレクトリにあるファイル
		files map[string]string
		// model・query・client は、設定で定義した出力のファイル名。空なら定義しない
		model  string
		query  string
		client string
		// bindPackages は、束縛先としてロードするパッケージの import パス
		bindPackages []string
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "すべての出力がGoとして読めればnilを返す",
			args: args{
				files: map[string]string{
					"model_gen.go":  "package domain\n\ntype Order struct{}\n",
					"query_gen.go":  "package domain\n\ntype GetOrder struct{}\n",
					"client_gen.go": "package query\n",
				},
				model:  "model_gen.go",
				query:  "query_gen.go",
				client: "client_gen.go",
			},
			want: want{
				err: nil,
			},
		},
		{
			// 境界値: 出力を 1 つも定義していない
			name: "定義していない出力は調べない",
			args: args{
				files: map[string]string{
					"model_gen.go": "package domain\n\ntype OrderFilter struct {\n\tCreatedAfter *invalid type\n}\n",
				},
			},
			want: want{
				err: nil,
			},
		},
		{
			// 使用フィルタがすべての型を除いた model ファイルは、生成の後に消してある
			name: "出力先にファイルが無い出力は調べない",
			args: args{
				files: map[string]string{
					"query_gen.go":  "package domain\n",
					"client_gen.go": "package query\n",
				},
				model:  "model_gen.go",
				query:  "query_gen.go",
				client: "client_gen.go",
			},
			want: want{
				err: nil,
			},
		},
		{
			// 束縛した型を解決できないと、gqlgen は型の位置に invalid type と書いたコードを出力する
			name: "読めないmodelは最初の構文エラーの位置とその行を返す",
			args: args{
				files: map[string]string{
					"model_gen.go": "package domain\n\ntype OrderFilter struct {\n\tCreatedAfter graphql.Omittable[*invalid type] `json:\"createdAfter\"`\n}\n",
					"query_gen.go": "package domain\n",
				},
				model: "model_gen.go",
				query: "query_gen.go",
			},
			want: want{
				err: errors.New("generated file is not valid Go: <dir>/model_gen.go:4:42: missing ',' in type argument list; line 4 is: CreatedAfter graphql.Omittable[*invalid type] `json:\"createdAfter\"`"),
			},
		},
		{
			name: "modelとqueryが読めてclientが読めないときはclientのエラーを返す",
			args: args{
				files: map[string]string{
					"model_gen.go":  "package domain\n",
					"query_gen.go":  "package domain\n",
					"client_gen.go": "package query\n\nfunc GetOrder() (*invalid type, error) {\n\treturn nil, nil\n}\n",
				},
				model:  "model_gen.go",
				query:  "query_gen.go",
				client: "client_gen.go",
			},
			want: want{
				err: errors.New("generated file is not valid Go: <dir>/client_gen.go:3:27: missing ',' in parameter list; line 3 is: func GetOrder() (*invalid type, error) {"),
			},
		},
		{
			// 境界値: 長さ 0 のファイル。エラーの位置の行が空なので、行は添えない
			name: "空のファイルは行を添えずにエラーを返す",
			args: args{
				files: map[string]string{
					"query_gen.go": "",
				},
				query: "query_gen.go",
			},
			want: want{
				err: errors.New("generated file is not valid Go: <dir>/query_gen.go:1:1: expected 'package', found 'EOF'"),
			},
		},
		{
			// 境界値: エラーの位置が、改行で終わらない最後の行にある
			name: "途中で切れたファイルは最後の行を添えてエラーを返す",
			args: args{
				files: map[string]string{
					"query_gen.go": "package domain\n\nfunc GetOrder(",
				},
				query: "query_gen.go",
			},
			want: want{
				err: errors.New("generated file is not valid Go: <dir>/query_gen.go:3:15: expected ')', found 'EOF'; line 3 is: func GetOrder("),
			},
		},
		{
			name: "出力先がディレクトリのときは読み込みのエラーを返す",
			args: args{
				files: map[string]string{
					"query_gen.go/": "",
				},
				query: "query_gen.go",
			},
			want: want{
				err: errors.New("read generated file: read <dir>/query_gen.go: is a directory"),
			},
		},
		{
			// 読めないコードが出る原因の手がかりとして、束縛先のパッケージの読み込みエラーを添える
			name: "束縛先のパッケージに読み込みエラーがあればエラーに添える",
			args: args{
				files: map[string]string{
					"query_gen.go": "package domain\n\ntype GetStatus struct {\n\tStatus invalid type `json:\"status\"`\n}\n",
				},
				query:        "query_gen.go",
				bindPackages: []string{unloadablePackage},
			},
			want: want{
				err: errors.New("generated file is not valid Go: <dir>/query_gen.go:4:17: expected ';', found 'type'; line 4 is: Status invalid type `json:\"status\"`; packages were loaded with errors: package " + unloadablePackage + " was loaded with errors: build constraints exclude all Go files in <repo>/testdata/integration/bind-unloadable-package/domain"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeOutputFiles(t, dir, tt.args.files)

			binder := typebind.New()
			if err := binder.Load(tt.args.bindPackages...); err != nil {
				t.Fatalf("failed to load bind packages: %v", err)
			}

			// output は、name を出力先のディレクトリのファイルにした出力の設定を返す。空なら定義しない
			output := func(name string) config.PackageConfig {
				if name == "" {
					return config.PackageConfig{}
				}

				return config.PackageConfig{Filename: filepath.Join(dir, name)}
			}
			cfg := &config.Config{
				ModelGen:  output(tt.args.model),
				QueryGen:  output(tt.args.query),
				ClientGen: output(tt.args.client),
			}

			err := validateOutputs(cfg, &packageCache{gqlgen: nil, binder: binder})

			if got := maskTestDirs(t, err, dir); !cmputil.EqualErrorMessage(tt.want.err, got) {
				t.Errorf("error:\nwant: %v\n got: %v", tt.want.err, got)
			}
		})
	}
}

func TestPackageErrorMessages(t *testing.T) {
	t.Parallel()

	type args struct {
		pkgErrs []error
	}

	type want struct {
		messages []string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 境界値: エラーが無い
			name: "エラーが無ければ空を返す",
			args: args{
				pkgErrs: nil,
			},
			want: want{
				messages: []string{},
			},
		},
		{
			// gqlgen は map の反復順でエラーを返すため、順を固定する
			name: "メッセージの昇順に並べて重複を除く",
			args: args{
				pkgErrs: []error{
					errors.New("order.go:12:9: undefined: OrderID"),
					errors.New("domain.go:5:2: could not import example.com/shop/money"),
					errors.New("order.go:12:9: undefined: OrderID"),
				},
			},
			want: want{
				messages: []string{
					"domain.go:5:2: could not import example.com/shop/money",
					"order.go:12:9: undefined: OrderID",
				},
			},
		},
		{
			// 境界値: 上限と同じ 3 件はすべて返し、残りの件数は付けない
			name: "3件のときはすべて返す",
			args: args{
				pkgErrs: []error{
					errors.New("order.go:12:9: undefined: OrderStatus"),
					errors.New("order.go:11:9: undefined: OrderNumber"),
					errors.New("order.go:10:9: undefined: OrderID"),
				},
			},
			want: want{
				messages: []string{
					"order.go:10:9: undefined: OrderID",
					"order.go:11:9: undefined: OrderNumber",
					"order.go:12:9: undefined: OrderStatus",
				},
			},
		},
		{
			name: "3件を超えるときは先頭の3件と残りの件数を返す",
			args: args{
				pkgErrs: []error{
					errors.New("order.go:14:9: undefined: CanceledAt"),
					errors.New("order.go:13:9: undefined: OrderedAt"),
					errors.New("order.go:12:9: undefined: OrderStatus"),
					errors.New("order.go:11:9: undefined: OrderNumber"),
					errors.New("order.go:10:9: undefined: OrderID"),
				},
			},
			want: want{
				messages: []string{
					"order.go:10:9: undefined: OrderID",
					"order.go:11:9: undefined: OrderNumber",
					"order.go:12:9: undefined: OrderStatus",
					"and 2 more",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := packageErrorMessages(tt.args.pkgErrs)

			if diff := cmp.Diff(tt.want.messages, got); diff != "" {
				t.Errorf("messages diff(-want +got): %s", diff)
			}
		})
	}
}
