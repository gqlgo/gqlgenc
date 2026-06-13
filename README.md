# gqlgenc

[gqlgen](https://github.com/99designs/gqlgen) をベースにした、Go 用 GraphQL クライアントのコードジェネレータおよびランタイムライブラリ。

## 概要

gqlgenc は、GraphQL スキーマとクエリ（オペレーション）から型安全な Go のクライアントコードを自動生成するツールです。コードを起点にする Code First ではなく、クエリを起点にコードを生成する Query First を採用しています。

- gqlgen ベース。モデル生成には gqlgen の modelgen プラグインをそのまま利用し、設定も gqlgen の形式を踏襲します
- 生成コードとランタイムは `encoding/json/v2` / `encoding/json/jsontext` を使用します

## v1 設計方針

- シンプルな生成コード
- シンプルな実装コード
- 最小のConfig、設定より規約
- モダンな言語機能の採用

## 動作要件

- Go 1.27 以上
- ビルド・テスト時に環境変数 `GOEXPERIMENT=jsonv2` の設定が必要です（`encoding/json/v2` を使用するため）

## インストール

```shell
go get -tool github.com/Yamashou/gqlgenc/v3@latest
# または
go install github.com/Yamashou/gqlgenc/v3@latest
```

## 使い方

### 1. 設定ファイルを書く

カレントディレクトリ（見つからない場合は親ディレクトリを順に遡る）の `.gqlgenc.yml` / `gqlgenc.yml` / `.gqlgenc.yaml` / `gqlgenc.yaml` を読み込みます。設定ファイルは `gqlgenc` と `gqlgen` の2セクションで構成されます。

ローカルのスキーマファイルから生成する例:

```yaml
gqlgenc:
  query:
    - ./query/*.graphql
  querygen:
    filename: ./domain/query_gen.go
  clientgen:
    filename: ./query/client_gen.go
gqlgen:
  schema:
    - ./schema/*.graphql
  model:
    filename: ./domain/model_gen.go
  enable_model_json_omitzero_tag: true
  nullable_input_omittable: true
  struct_fields_always_pointers: false
  autobind:
    - github.com/example/myapp/domain
  models:
    Email:
      model: github.com/example/myapp/domain.Email
```

リモートサーバーからイントロスペクションでスキーマを取得する場合は、`gqlgen.schema` の代わりに `gqlgenc.endpoint` を指定します。

```yaml
gqlgenc:
  endpoint:
    url: https://api.example.com/graphql
    headers:
      Authorization: "Bearer ${TOKEN}" # 環境変数を展開できる
  query:
    - ./query/*.graphql
  querygen:
    filename: ./gen/query_gen.go
  clientgen:
    filename: ./gen/client_gen.go
gqlgen:
  model:
    filename: ./gen/model_gen.go
```

### 2. コードを生成する

設定ファイルのあるディレクトリで実行します。

```shell
gqlgenc
```

### 3. 生成されたクライアントを使う

```go
package main

import (
	"context"

	"github.com/Yamashou/gqlgenc/v3/client"

	"github.com/example/myapp/query"
)

func main() {
	ctx := context.Background()

	c := client.NewClient("https://api.example.com/graphql")

	res, err := client.Do(ctx, c, query.GetUserOp, query.GetUserVars{ID: "user-1"})
	if err != nil {
		// エラー処理
	}
	_ = res
}
```

動作する完全なサンプルは `testdata/integration/basic/` と `run_test.go` を参照してください。

## 設定仕様

設定ファイルは YAML 形式で、読み込み時に `os.ExpandEnv` によって環境変数（`${VAR}` / `$VAR`）が展開されます。未知のキーがあるとエラーになります。

### gqlgenc セクション

| キー | 型 | 説明 |
|---|---|---|
| `query` | `[]string` | クエリファイルのパス（glob 可） |
| `querygen` | `filename` / `package` | レスポンス型・UnmarshalJSON・クエリドキュメント定数の生成先。`package` 省略時はディレクトリ名から導出 |
| `clientgen` | `filename` / `package` | 型付き variables 構造体と `client.Operation` 値の生成先。指定する場合は `querygen` の指定も必須 |
| `endpoint` | `url` / `headers` | イントロスペクションでスキーマを取得するエンドポイント。`gqlgen.schema` と排他 |
| `export_query_type` | `bool` | ネストしたレスポンス型の型名を公開する（`UserOperation_User` 形式）。デフォルトの false では先頭が小文字の非公開型（`userOperation_User`）になる |

### gqlgen セクション

gqlgen の `codegen/config.Config` をそのまま埋め込んでいるため、gqlgen と同じ設定が使えます。主なもの:

| キー | 説明 |
|---|---|
| `schema` | スキーマファイルのパス（glob 可、`**` 対応）。`gqlgenc.endpoint` と排他 |
| `model` | gqlgen modelgen による model_gen.go の生成先 |
| `models` | GraphQL 型と Go 型のバインド設定 |
| `autobind` | 指定パッケージから同名の Go 型を自動バインド |
| `federation.version` | Apollo Federation ディレクティブを含むスキーマへの対応 |
| `enable_model_json_omitzero_tag` | モデルの json タグに omitzero を付与 |
| `nullable_input_omittable` | nullable な input フィールドを `graphql.Omittable` にして null / undefined を区別 |
| `struct_fields_always_pointers` | モデルのフィールドを常にポインタにするか |

なお `directives` / `exec` / `resolver` / `federation` の生成先設定はクライアント生成では使用しないため、内部で固定値に上書きされます。

### バリデーションルール

- `gqlgen.schema` と `gqlgenc.endpoint` はどちらか一方を必ず指定する（両方指定・両方未指定はエラー）
- `clientgen` を指定する場合は `querygen` の指定が必須
- クエリ全体でオペレーション名が重複しているとエラー

## 生成されるコード

生成実行時、既存の生成ファイル（model / querygen / clientgen）は事前に削除されます。生成後は goimports で整形されます。

### model_gen.go（modelgen）

gqlgen 標準の modelgen プラグインで input 型・enum などを生成します。gqlgenc は MutateHook で次のフィルタリングを行います。

- `querygen` または `clientgen` が定義されている場合: すべての型を生成する（レスポンスのデシリアライズに必要なため）
- どちらも未定義の場合: Input 型は常に生成し、Object 型と Enum 型はクエリで使用されているものだけを生成する。使用判定はオペレーションの変数定義（Input 型はフィールドを再帰的に辿る）とセレクションセット（レスポンスフィールドの enum）から行う

enum には gqlgen が `MarshalJSON` / `UnmarshalJSON` を生成するため（[gqlgen#3663](https://github.com/99designs/gqlgen/pull/3663)）、json.Marshal でそのままシリアライズできます。

### query_gen.go（querygen）

オペレーションごとに以下を生成します。

- レスポンス型: セレクションセットの構造に対応したネスト構造体。各フィールドには json タグと、nil レシーバ安全な getter メソッドが付く
- フラグメント対応:
  - フラグメントスプレッドは `json:"-"` 付きの埋め込み構造体として表現し、UnmarshalJSONFrom 内で同じ JSON データから直接デコードする
  - インラインフラグメント（`... on Type`）は `__typename` の値を見て対応するフィールドにデコードする
- `UnmarshalJSONFrom`（json/v2 の `UnmarshalerFrom`）はフラグメントを含む型にのみ生成される。通常フィールドはメソッドを持たない別名型（`type plain T`）を経由して json/v2 のデフォルトデコードに任せ、フラグメントスプレッドと `__typename` 分岐だけを追加でデコードする。フラグメントを含まない型はメソッド自体を生成せず、デフォルトデコードで処理される
- クエリドキュメント定数（`<オペレーション名>Document`）

### client_gen.go（clientgen）

オペレーションごとに型付き variables 構造体（`<オペレーション名>Vars`）と `client.Operation` 値（`<オペレーション名>Op`）を生成します。実行は `client.Do` で行い、変数名や型のミスはコンパイルエラーになります。

```go
type UserOperationVars struct {
	ArticleID string `json:"articleId"`
	Size      *int   `json:"size"`
}

var UserOperationOp = client.Operation[UserOperationVars, domain.UserOperation]{
	Name:     "UserOperation",
	Document: domain.UserOperationDocument,
}
```

```go
res, err := client.Do(ctx, c, query.UserOperationOp, query.UserOperationVars{
	ArticleID: "article-1",
	Size:      &size,
})
```

## ランタイム（client パッケージ）

### API

- `client.NewClient(endpoint string, options ...Option) *Client`
- `client.WithHTTPClient(*http.Client)` — 任意の HTTP クライアントを使用する
- `client.WithHTTPHeader(http.Header)` — すべてのリクエストに付与するヘッダーを設定する（デフォルトヘッダーとはキー単位でマージされ、同名キーは上書きされる）
- `client.Operation[Vars, Res]` / `client.Do(ctx, c, op, vars, options...)` — clientgen が生成する Operation 値を型付き variables で実行する
- `(*Client).Post(ctx, operationName, query, variables, out, options...)` — map 変数でオペレーションを実行する低レベル API

### リクエスト仕様

- HTTP POST で送信する
- `Content-Type: application/json;charset=utf-8`
- `Accept: application/graphql-response+json;charset=utf-8` と `application/json;charset=utf-8`
- ボディは `{"query": ..., "variables": ..., "operationName": ...}` を json/v2 でエンコードしたもの

### ファイルアップロード

variables に `graphql.Upload`（`github.com/99designs/gqlgen/graphql`）が含まれる場合、`Do` / `Post` のどちらでも [graphql-multipart-request-spec](https://github.com/jaydenseric/graphql-multipart-request-spec) に従った multipart/form-data リクエストを自動的に組み立てます。Upload は variables のエンコード中に検出されるため、ネストした input オブジェクトやリストの中にあっても動作します。Upload が含まれない場合は通常の JSON リクエストになります。

### レスポンス解析とエラー

- `Content-Encoding: gzip` のレスポンスは透過的に展開される
- レスポンスボディを1パスで走査し、`data` は生成されたレスポンス型へ直接デコードし、`errors` は `gqlerror.List`（vektah/gqlparser）としてデコードする
- エラー時は `ErrorResponse` が返り、`errors.As` で `*client.HTTPError`（HTTP ステータス異常）や `gqlerror.List`（GraphQL エラー）を取り出せる
- GraphQL エラー時もデコード済みの部分データが `client.Do` の戻り値として返る（GraphQL は data と errors の共存を許すため）
- HTTP ステータスが 2xx でも GraphQL レスポンスとしてパースできない場合はエラーになる

### null と undefined の区別

`nullable_input_omittable: true` を設定すると、input 型の nullable フィールドが `graphql.Omittable[T]` になり、null と undefined を区別して送信できます。

```go
// undefined: キー自体が送信されない
input := UpdateUserInput{
	Name: graphql.Omittable[*string]{},
}

// null: {"name": null} が送信される
input := UpdateUserInput{
	Name: graphql.OmittableOf[*string](nil),
}
```

## アーキテクチャと処理フロー

`gqlgenc` コマンドは次の順に処理します。

1. **設定読み込み**（`config.LoadConfig`）: 設定ファイルの探索、YAML パース（環境変数展開・未知キー検出）、バリデーション
2. **スキーマ読み込み**（`config.LoadSchema`）: ローカルファイルまたはイントロスペクションでスキーマを AST 化し、既存の生成ファイルを削除した上で gqlgen の `Init()` を実行する。interface の実装リストをソートして出力を決定的にする
3. **クエリ読み込み**（`GQLGencConfig.LoadQuery`）: クエリファイルをパースしてスキーマに対して検証し、オペレーション単位の QueryDocument（参照フラグメント込み）に分割する
4. **コード生成**（`plugins.GenerateCode`）: modelgen → オペレーションと Go 型の構築（codegen）→ querygen → clientgen の順に実行する

### パッケージ構成

| パッケージ | 役割 |
|---|---|
| `main` / `run.go` | エントリポイント。上記フローの実行 |
| `config` | 設定ファイルの読み込み・検証、スキーマのロード |
| `introspection` | イントロスペクション結果から GraphQL スキーマ（AST）を構築 |
| `queryparser` | クエリのパース・検証、オペレーション分割、使用型の収集 |
| `codegen` | オペレーションと Go 型（go/types）の構築 |
| `plugins/modelgen` | gqlgen modelgen のラップ（未使用型のフィルタリング） |
| `plugins/querygen` | レスポンス型・UnmarshalJSON・ドキュメント定数の生成 |
| `plugins/clientgen` | 型付き Operation 値の生成 |
| `client` | ランタイムの HTTP クライアント |

## 開発

```shell
make build # go build ./...
make test  # go test ./...
make lint  # golangci-lint run
make fmt   # golangci-lint fmt
```

Makefile が `GOEXPERIMENT=jsonv2` をエクスポートします。

## 類似プロジェクト

- [Khan/genqlient](https://github.com/Khan/genqlient) — 独自実装の GraphQL クライアントジェネレータ。gqlgenc は gqlgen ベースのため、gqlgen の知識・設定をそのまま活かせる点が異なります
