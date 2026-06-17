package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

const version = "1.0.0-alpha1"

var versionOption = flag.Bool("version", false, "gqlgenc version")

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "gqlgenc は GraphQL スキーマとクエリから型安全な Go クライアントを生成します。")
		fmt.Fprintln(os.Stderr, "カレントディレクトリ（見つからなければ親を順に遡って）見つかる .gqlgenc.yml を読み、設定どおりにコードを生成します。")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  gqlgenc            設定ファイルのあるディレクトリで実行し、コードを生成する")
		fmt.Fprintln(os.Stderr, "  gqlgenc -version   バージョンを表示する")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *versionOption {
		fmt.Printf("gqlgenc v%s", version)

		return
	}

	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
