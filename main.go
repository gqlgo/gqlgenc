package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
)

// version はビルド時に -ldflags "-X main.version=v3.0.0" で設定できる。空のときは、ビルド情報に
// 記録されたモジュールのバージョン (go install や go tool で入れたときに要求されたバージョン) を使う。
// 定数で持つと、リリースのたびに書き換えを忘れて実際のバージョンと食い違う。
var version = ""

var versionOption = flag.Bool("version", false, "gqlgenc version")

// resolveVersion は -version で表示するバージョンを返す。override が空でなければそれを、
// 空ならビルド情報の main モジュールのバージョンを返す。ビルド情報が無いか、バージョンが
// 記録されていなければ "(devel)" を返す。空文字は返さない。
func resolveVersion(override string) string {
	if override != "" {
		return override
	}

	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}

	return "(devel)"
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "gqlgenc は GraphQL スキーマとクエリから型安全な Go クライアントを生成します。")
		fmt.Fprintln(os.Stderr, "カレントディレクトリ（見つからなければ親を順に遡って）見つかる .gqlgenc.yml を読み、設定どおりにコードを生成します。")
		fmt.Fprintln(os.Stderr, "設定ファイルを引数で指定した場合、設定内の相対パスは各設定ファイルのディレクトリを基準に解決します。")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  gqlgenc                     設定ファイルのあるディレクトリで実行し、コードを生成する")
		fmt.Fprintln(os.Stderr, "  gqlgenc <設定ファイル>...    指定した設定ファイルを順に処理する (パッケージ情報のロードを共有するため複数プロセスより高速)")
		fmt.Fprintln(os.Stderr, "  gqlgenc -version            バージョンを表示する")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *versionOption {
		fmt.Println("gqlgenc " + resolveVersion(version))

		return
	}

	ctx := context.Background()
	if err := run(ctx, flag.Args()...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
