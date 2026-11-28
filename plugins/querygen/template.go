// Package querygen は、オペレーションのレスポンスの型を query_gen.go に生成する。
//
// codegen が作ったレスポンスの型 (codegen.ResponseType) を読み、型の宣言と、フラグメントを持つ型の
// UnmarshalJSONFrom / MarshalJSONTo を書く。選択の種類 (フィールド・フラグメントスプレッド・
// インラインフラグメント) は codegen が決めていて、このパッケージは判定し直さない。
//
// 生成するコードは json/v2 を使う。フラグメントのフィールド (json:"-") は既定のデコードでは値が
// 入らないため、フラグメントを持つ型にだけ UnmarshalJSONFrom を生成し、同じ JSON オブジェクトから
// フラグメントスプレッドと、__typename が型条件を満たすインラインフラグメントをデコードする。
package querygen

import (
	"bytes"
	_ "embed" // used to load template file
	"errors"
	"fmt"
	"slices"
	"text/template"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/config"
	"github.com/Yamashou/gqlgenc/v3/internal/gosrc"
)

//go:embed template.tmpl
var templateText string

// Generate は、レスポンスの型 responseTypes とオペレーションの Document 定数を、設定の
// generate.query.file へ書き出す。型ごとのコードは genCode が作る。
func Generate(cfg *config.Config, operations []*codegen.Operation, responseTypes []*codegen.ResponseType) error {
	imports := gosrc.NewImports(cfg.QueryGen.ImportPath)

	// UnmarshalJSONFrom / MarshalJSONTo は、次のパッケージを名前で参照する。生成する型が 1 つでもあれば
	// import する。型の束縛先のパッケージより先に名前を押さえ、束縛先のパッケージが同じ名前 (client など)
	// でも、こちらの名前を変えない。
	if slices.ContainsFunc(responseTypes, hasFragment) {
		if err := errors.Join(
			imports.Reserve("encoding/json/jsontext", "jsontext"),
			imports.Reserve("encoding/json/v2", "json"),
			imports.Reserve("github.com/Yamashou/gqlgenc/v3/client", "client"),
		); err != nil {
			return fmt.Errorf("%s generating failed: %w", cfg.QueryGen.Filename, err)
		}
	}

	tmpl, err := template.New("querygen").Funcs(template.FuncMap{
		"go": gosrc.ToGo,
		"genCode": func(responseType *codegen.ResponseType) string {
			return genCode(responseType, cfg.GenerateGetters, imports)
		},
	}).Parse(templateText)
	if err != nil {
		return fmt.Errorf("%s generating failed: %w", cfg.QueryGen.Filename, err)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, map[string]any{
		"ResponseTypes": responseTypes,
		"Operations":    operations,
	}); err != nil {
		return fmt.Errorf("%s generating failed: %w", cfg.QueryGen.Filename, err)
	}

	if err := gosrc.WriteFile(cfg.QueryGen.Filename, cfg.QueryGen.Package, imports, body.Bytes()); err != nil {
		return fmt.Errorf("%s generating failed: %w", cfg.QueryGen.Filename, err)
	}

	return nil
}
