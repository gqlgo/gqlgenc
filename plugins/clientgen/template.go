// Package clientgen は、オペレーションごとの変数の型と Operation の値を client_gen.go に生成する。
package clientgen

import (
	"bytes"
	_ "embed" // used to load template file
	"errors"
	"fmt"
	"text/template"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/internal/gosrc"
)

//go:embed template.tmpl
var templateText string

// Generate は、オペレーション operations の変数の型 (<Name>Vars) と Operation の値 (<Name>Op) を、
// 設定の generate.client.file へ書き出す。
func Generate(cfg *config.Config, operations []*codegen.Operation) error {
	// レスポンスの型と Document 定数は generate.query.file のパッケージにある。出力先が同じパッケージなら
	// 修飾せずに参照する
	samePackage := cfg.ClientGen.ImportPath == cfg.QueryGen.ImportPath

	// テンプレートが名前で参照するパッケージ。使わなかったものは書き出されない。
	// query が同じパッケージのときは、Reserve が import に加えない
	imports := gosrc.NewImports(cfg.ClientGen.ImportPath)
	if err := errors.Join(
		imports.Reserve("github.com/Yamashou/gqlgenc/v3/client", "client"),
		imports.Reserve("github.com/99designs/gqlgen/graphql", "graphql"),
		imports.Reserve(cfg.QueryGen.ImportPath, cfg.QueryGen.Package),
	); err != nil {
		return fmt.Errorf("%s generating failed: %w", cfg.ClientGen.Filename, err)
	}

	tmpl, err := template.New("clientgen").Funcs(template.FuncMap{
		"go":  gosrc.ToGo,
		"ref": imports.TypeString,
	}).Parse(templateText)
	if err != nil {
		return fmt.Errorf("%s generating failed: %w", cfg.ClientGen.Filename, err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, map[string]any{
		"QueryPackage": cfg.QueryGen.Package,
		"SamePackage":  samePackage,
		"Operations":   operations,
	}); err != nil {
		return fmt.Errorf("%s generating failed: %w", cfg.ClientGen.Filename, err)
	}

	if err := gosrc.WriteFile(cfg.ClientGen.Filename, cfg.ClientGen.Package, imports, body.Bytes()); err != nil {
		return fmt.Errorf("%s generating failed: %w", cfg.ClientGen.Filename, err)
	}

	return nil
}
