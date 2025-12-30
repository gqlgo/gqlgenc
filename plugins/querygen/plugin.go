// Package querygen はカスタム UnmarshalJSON メソッドを持つ GraphQL クライアントのクエリ型を生成する。
//
// このパッケージは GraphQL オペレーション（クエリ、ミューテーション、サブスクリプション）用の
// Go 型を生成するコードジェネレータプラグインを提供する。型安全な UnmarshalJSON メソッドは
// 以下の機能を処理する:
//   - Fragment spreads (json:"-" を持つ埋め込みフィールド)
//   - Inline fragments (__typename に基づく型条件付きフィールド)
//   - ネストしたフィールド構造
//
// 生成されるコードは json/v2 (GOEXPERIMENT=jsonv2) を使用し、jsontext.Value による
// 効率的な JSON アンマーシャリングで不要なアロケーションを回避する。
package querygen

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/imports"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin"

	"github.com/Yamashou/gqlgenc/v3/codegen"
	"github.com/Yamashou/gqlgenc/v3/config"
)

var _ plugin.ConfigMutator = &Plugin{}

// Plugin は GraphQL クライアントのクエリ型を生成するための gqlgen プラグインインターフェースを実装する。
// オペレーション型の型定義、UnmarshalJSON メソッド、getter メソッドを生成する。
type Plugin struct {
	cfg        *config.Config
	operations []*codegen.Operation
	goTypes    []types.Type
}

// New は新しい querygen プラグインインスタンスを作成する。
//
// パラメータ:
//   - cfg: gqlgenc の設定
//   - operations: パース済みの GraphQL オペレーション（クエリ、ミューテーション、サブスクリプション）
//   - goTypes: オペレーション用に生成された Go 型
//
// コード生成の準備ができた Plugin を返す。
func New(cfg *config.Config, operations []*codegen.Operation, goTypes []types.Type) *Plugin {
	return &Plugin{
		cfg:        cfg,
		operations: operations,
		goTypes:    goTypes,
	}
}

// Name は gqlgen のプラグインシステム用にこのプラグインの名前を返す。
func (p *Plugin) Name() string {
	return "querygen"
}

// MutateConfig は gqlgen の ConfigMutator インターフェースを実装する。
// クエリ型ファイルを生成し、goimports を実行する。
func (p *Plugin) MutateConfig(_ *gqlgenconfig.Config) error {
	if err := RenderTemplate(p.cfg, p.operations, p.goTypes); err != nil {
		return fmt.Errorf("template failed: %w", err)
	}

	if _, err := imports.Process(p.cfg.GQLGencConfig.QueryGen.Filename, nil, nil); err != nil {
		return fmt.Errorf("go imports: %w", err)
	}

	return nil
}
