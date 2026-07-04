# v0 から v1 への移行ガイド

v1.0.0-alpha1 は v0.x からの全面書き直しです。変更の全体像は [CHANGELOG.md](./CHANGELOG.md) を参照してください。

## 1. 動作要件とモジュールパス

- Go 1.27 以上が必要です。ビルド・テストには環境変数 `GOEXPERIMENT=jsonv2` の設定が必要です
- import パスを `github.com/Yamashou/gqlgenc` から `github.com/Yamashou/gqlgenc/v3` に変更してください

## 2. 設定ファイル（.gqlgenc.yml）

v0 の設定例:

```yaml
model:
  package: generated
  filename: ./models_gen.go
client:
  package: generated
  filename: ./client.go
models:
  Int:
    model: github.com/99designs/gqlgen/graphql.Int64
federation:
  version: 2
endpoint:
  url: https://api.example.com/graphql
  headers:
    Authorization: "Bearer ${TOKEN}"
query:
  - "./query/*.graphql"
generate:
  clientInterfaceName: "GraphQLClient"
  structFieldsAlwaysPointers: false
  onlyUsedModels: true
```

v1 の設定例（ローカルスキーマ）:

```yaml
schema:
  files:
    - ./schema/*.graphql
query:
  files:
    - ./query/*.graphql
bind:
  type:
    named:
      Email:
        model: github.com/example/myapp/domain.Email
generate:
  model:
    file: ./domain/model_gen.go
  query:
    file: ./domain/query_gen.go
  client:
    file: ./query/client_gen.go
```

v0 で `endpoint` を使っていた場合は、v1 では `schema.endpoint` に移動します。

```yaml
schema:
  endpoint:
    url: https://api.example.com/graphql
    headers:
      Authorization: "Bearer ${TOKEN}" # 環境変数を展開できる
query:
  files:
    - ./query/*.graphql
generate:
  model:
    file: ./gen/model_gen.go
  query:
    file: ./gen/query_gen.go
  client:
    file: ./gen/client_gen.go
```

### v0 → v1 設定対応表

| v0 | v1 |
|---|---|
| `schema`（トップレベルの配列） | `schema.files` |
| `endpoint.url` / `endpoint.headers` | `schema.endpoint.url` / `schema.endpoint.headers` |
| `federation.version` | `schema.federation.version` |
| `query`（トップレベルの配列） | `query.files` |
| `model.filename` | `generate.model.file`（クエリで使われる Input / Enum のみ生成。サーバーとモデルを共有する場合は省略） |
| `client.filename` | `generate.client.file` |
| （相当なし） | `generate.query.file`（レスポンス型の出力先。新設） |
| `model.package` / `client.package` | 廃止（package は file のディレクトリ名から導出） |
| `autobind` | `bind.type.packages` |
| `models` | `bind.type.named` |
| （相当なし） | `bind.fragment.packages`（フラグメント名の自動バインド。新設） |
| `generate` 配下のトグル | すべて廃止（下記） |

廃止された `generate` 配下のトグルは次のとおりです。

- `prefix` / `suffix` / `unamedPattern`（生成型名の命名カスタマイズ）
- `client` / `clientInterfaceName`（クライアント生成のトグルと interface 名）
- `clientV2`
- `enableClientJsonOmitemptyTag` / `enableClientJsonOmitzeroTag`（JSON タグのトグル。v1 は `omitzero` 固定）
- `structFieldsAlwaysPointers`
- `nullableInputOmittable`
- `onlyUsedModels`（v1 では既定動作）

json/v2 タグや `graphql.Omittable`・`omitzero` など v1 が常に必要とする gqlgen 設定は内部で固定されます（`nullable_input_omittable` / `enable_model_json_omitzero_tag` / `struct_fields_always_pointers` などのトグルは廃止）。

設定ファイルに未知のフィールドがあるとエラーになります。v0 のキーが残っていると起動時に検出できます。

## 3. CLI

- v0: `gqlgenc generate --configdir schemas`
- v1: `.gqlgenc.yml` のあるディレクトリで `gqlgenc` を実行（サブコマンドと `--configdir` は廃止、フラグは `-version` のみ）

## 4. クライアント呼び出しコード

v0 は clientgen がオペレーションごとのメソッドを持つ `Client` 構造体を生成していました。

```go
c := generated.NewClient(http.DefaultClient, "https://api.example.com/graphql", nil)
res, err := c.GetUser(ctx, "user-1")
```

v1 は `client.Operation` 値と型付き variables 構造体を生成し、ランタイムのジェネリックメソッドで実行します。

```go
c := client.NewClient("https://api.example.com/graphql")
res, err := c.Post(ctx, query.GetUserOp, query.GetUserVars{ID: "user-1"})
```

query オペレーションは `c.Get` で HTTP GET 実行もできます。`client.Operation` の `Kind` 型パラメータにより、`Get` に mutation を渡すとコンパイルエラーになります。

## 5. Interceptor から http.RoundTripper へ

v0 の `RequestInterceptor`:

```go
type RequestInterceptor func(ctx context.Context, req *http.Request,
    gqlInfo *clientv2.GQLRequestInfo, res any, next clientv2.RequestInterceptorFunc) error
```

v0 でヘッダーを付与する典型例:

```go
authInterceptor := func(ctx context.Context, req *http.Request,
    gqlInfo *clientv2.GQLRequestInfo, res any, next clientv2.RequestInterceptorFunc) error {
    req.Header.Set("Authorization", "Bearer "+token)
    return next(ctx, req, gqlInfo, res)
}
c := clientv2.NewClient(http.DefaultClient, endpoint, nil, authInterceptor)
```

v1 では `Option`（`func(*http.Client) *http.Client`）と自前の `http.RoundTripper` で置き換えます。gqlgenc は `WithRoundTripper` / `WithHTTPClient` / `WithHTTPHeader` などのヘルパーを一切提供しません。

```go
// ヘッダー付与・認証などは自前の http.RoundTripper で行う。RoundTripper は
// リクエストごとに呼ばれるため、ローテーションするトークンや ctx 由来の値にも対応できる
type authTransport struct{ base http.RoundTripper }

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req = req.Clone(req.Context()) // RoundTripper は元のリクエストを変更しない
    req.Header.Set("Authorization", "Bearer "+tokenFrom(req.Context()))
    return t.base.RoundTrip(req)
}

// gqlgenc はヘルパーを提供しないので、transport を包む Option は自分で書く。
// 共有 http.Client を汚さないようコピーしてから transport を差し替える。
withTransport := func(wrap func(http.RoundTripper) http.RoundTripper) client.Option {
    return func(c *http.Client) *http.Client {
        base := c.Transport
        if base == nil {
            base = http.DefaultTransport
        }
        cc := *c
        cc.Transport = wrap(base)
        return &cc
    }
}

c := client.NewClient(endpoint, withTransport(func(base http.RoundTripper) http.RoundTripper {
    return authTransport{base: base}
}))

// 呼び出し単位で付与する場合は Post / Get / Subscribe にオプションとして渡す
c.Post(ctx, op, vars, withTransport(func(base http.RoundTripper) http.RoundTripper {
    return authTransport{base: base}
}))
```

v0 の `RequestInterceptor`（`func(ctx, req, gqlInfo, res, next) error`）の各引数との対応は次のとおりです。

- `ctx` / `req` — `req`（と `req.Context()`）をそのまま使う
- `gqlInfo`（パース済みの Query / Variables / OperationName）— transport が見るのはシリアライズ後の JSON ボディ。必要であればボディを `{"query": ..., "variables": ..., "operationName": ...}` としてデコードして参照する
- `res`（デコード済みレスポンス）— デコードは transport の後段で行われるため、型付きの `res` は transport からは見えない。レスポンスを加工したい場合はレスポンスボディの JSON を書き換えれば、その後のデコード結果に反映される
- `ChainInterceptor` — `withTransport` で包んだ Option を `NewClient` に複数渡すことで代替する。後に渡したものが外側（先に実行される側）になる

詳細は README の「HTTP のカスタマイズ（ヘッダー・認証）」セクションを参照してください。

## 6. エラーハンドリング

v0 では `clientv2.ErrorResponse`（`NetworkError *HTTPError` / `GqlErrors *GqlErrorList`）を `errors.As` で取り出していました。

v1 では `GqlErrorList` は廃止されました。`errors.As` で `*client.HTTPError` と `gqlerror.List`（`github.com/vektah/gqlparser/v2/gqlerror`）を直接取り出します。

```go
res, err := c.Post(ctx, query.GetUserOp, query.GetUserVars{ID: "user-1"})
if err != nil {
	// HTTP ステータス異常・GraphQL エラーは errors.As で型別に取り出せる
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		fmt.Println("http status:", httpErr.Code)
	}
	var gqlErrs gqlerror.List
	if errors.As(err, &gqlErrs) {
		fmt.Println("graphql errors:", gqlErrs)
	}
	// GraphQL エラー時も res に部分データが入っていることがある
}
```

GraphQL エラー時も部分データが `res` に入ります。

## 7. 生成されるレスポンス型の変化

移行時に影響が出る主な点は次のとおりです。

- フラグメントは埋め込みから名前付きフィールドになりました。アクセスが `t.Name` から `t.UserFragment1.Name` のように変わります
- インラインフラグメントは `親型名_型条件` の名前付き型（例: `UserOperation_User_User`）へのポインタフィールドになり、`__typename` が一致した場合のみ非 nil です。利用側は nil チェックが必要です
- レスポンス型は常に公開型です（アンダースコア区切りの型名）
- nil セーフ getter は既定で生成されません。必要なら `generate.query.getters: true` を指定してください
- 構造体タグから `graphql` タグが消え、`json` タグのみになりました

GraphQL クエリと v0 / v1 の生成コード比較（UserOperation の例）:

```graphql
query UserOperation {
  user {
    ... on User {
      ...UserFragment1
    }
    ...UserFragment1
  }
}

fragment UserFragment1 on User {
  name
}
```

v1.0.0-alpha1 の生成コード:

```go
type UserOperation struct {
	User UserOperation_User `json:"user"`
}

type UserOperation_User struct {
	User          *UserOperation_User_User `json:"-"`
	UserFragment1 UserFragment1            `json:"-"`
}

type UserOperation_User_User struct {
	UserFragment1 UserFragment1 `json:"-"`
}

type UserFragment1 struct {
	Name string `json:"name"`
}
```

v0.32.0 の生成コード:

```go
type UserOperation struct {
	User UserOperation_User `json:"user" graphql:"user"`
}

type UserOperation_User struct {
	User UserOperation_User_User `graphql:"... on User"`
	Name string                  `json:"name" graphql:"name"`
}

type UserOperation_User_User struct {
	Name string `json:"name" graphql:"name"`
}
```

## 8. ワイヤー上の挙動変更

- nil スライスは `[]`、nil マップは `{}` として送信されます（v0 は `null`）。`null` を送るにはポインタ型（nil の `*[]T`）を使うか、独自構造体ではフィールドに `format:emitnull` タグを指定してください
- undefined / null / 値の3状態を `graphql.Omittable` + `omitzero` で表現できるようになりました（nullable な変数・input フィールド）
- クエリドキュメントはミニファイされて送信されます
- `Accept` ヘッダーで `application/graphql-response+json` をネゴシエーションします
- `graphql.Upload` はネストした input やリスト内でも自動で multipart になります

## 9. Subscription（v1 で新規対応）

v0 は未対応でした。v1 では `NewSubscriptionClient` に WebSocket URL（`ws://` / `wss://`）を渡し、`Subscribe` が `iter.Seq2` を返します。[graphql-transport-ws](https://github.com/enisdenjo/graphql-ws) プロトコルを使用します。

```go
c := client.NewSubscriptionClient("wss://api.example.com/graphql")

for res, err := range c.Subscribe(ctx, query.OnMessageOp, query.OnMessageVars{RoomID: "1"}) {
	if err != nil {
		// エラー処理
		break
	}
	fmt.Println(res.OnMessage.Body)
}
```

query / mutation と同じ `client.Operation` 値（`<オペレーション名>Op`）をそのまま使えます。
