# Changelog

## v1.0.0-alpha1

v0.x からの全面的な書き直しです。変更の全体像は [gqlgo/gqlgenc#281](https://github.com/gqlgo/gqlgenc/pull/281) を参照してください。

### 動作要件

- Go 1.27 以上が必要です。ビルド・テストには環境変数 `GOEXPERIMENT=jsonv2` の設定が必要です
- モジュールパスを `github.com/Yamashou/gqlgenc` から `github.com/Yamashou/gqlgenc/v3` に変更しました

### 破壊的変更

#### JSON 処理を encoding/json/v2 に全面移行

- JSON のエンコード・デコードを `encoding/json/v2` / `encoding/json/jsontext` に全面移行しました
- 独自のランタイムデコーダだった `graphqljson` パッケージを廃止しました。レスポンスのデコードは querygen が生成する型ごとの `UnmarshalJSONFrom` が担います（「新機能」参照）
- v0 のクライアントが持っていた独自の JSON エンコード処理（`MarshalJSON`）を廃止し、リクエストボディは `json.MarshalWrite` で直接エンコードします。`graphql.Omittable` のエンコードは gqlgen 本体の対応（[99designs/gqlgen#3659](https://github.com/99designs/gqlgen/pull/3659)、[#3660](https://github.com/99designs/gqlgen/pull/3660)、[#3663](https://github.com/99designs/gqlgen/pull/3663)、[#3675](https://github.com/99designs/gqlgen/pull/3675)）と json/v2 の `omitzero` を利用します
- json/v2 のデフォルト挙動の変更により、リクエストの variables などのエンコードで nil スライスは `null` ではなく `[]`、nil マップは `{}` として送信されます（v1 json を使う v0 は `null` を送信していました）。GraphQL では `null` と空リスト `[]` をサーバーが区別し得るため注意してください。`null` を送りたい場合はポインタ型（nil の `*[]T`）を使うか、独自構造体ではフィールドに `format:emitnull` タグを指定してください

#### 設定ファイルを gqlgenc: / gqlgen: の2セクション構成に変更

- gqlgenc 固有の設定と gqlgen 由来の設定を分離しました。gqlgen の設定（`schema` / `model` / `models` / `autobind` / `federation` など）は `gqlgen:` セクションにそのまま記述します
- 旧 `generate:` セクションのオプション（`clientInterfaceName` / `onlyUsedModels` など）は廃止しました
- 設定ファイルに未知のフィールドがあるとエラーになります

```yaml
gqlgenc:
  query:
    - ./query/*.graphql
  querygen:
    filename: ./domain/query_gen.go
  clientgen:
    filename: ./query/client_gen.go
  export_query_type: true

gqlgen:
  schema:
    - ./schema/*.graphql
  model:
    filename: ./domain/model_gen.go
```

- リモートスキーマを使う場合は `gqlgenc.endpoint` に URL とヘッダーを指定します（introspection で取得）。`schema` と `endpoint` の同時指定はエラーです

#### コード生成を querygen と clientgen に分割

- querygen: オペレーションごとのレスポンス型、`UnmarshalJSONFrom`（フラグメントを含む型のみ）、nil 安全な Getter、クエリドキュメント定数（`<オペレーション名>Document`）を生成します
- clientgen: 型付き variables 構造体（`<オペレーション名>Vars`）と `client.Operation` 値（`<オペレーション名>Op`）を生成します
- `clientgen` を使う場合は `querygen` の指定が必須です。出力先は別パッケージにできます（例: レスポンス型は domain パッケージ、クライアントは query パッケージ）
- 旧 `clientgenv2` / `generator` / `parsequery` / `querydocument` パッケージは `plugins`（modelgen / querygen / clientgen）/ `codegen` / `queryparser` に再編しました
- 生成後のファイルには goimports を適用します

#### ランタイムを client パッケージに刷新（clientv2 廃止）

- `NewClient(client HttpClient, baseURL string, options *Options, interceptors ...RequestInterceptor)` は `NewClient(endpoint string, options ...Option)` になりました
- `RequestInterceptor` と `NewClientWithUnsafeRequestInterceptor` を廃止しました。ヘッダーの付与は `WithHTTPHeader`、それ以外のカスタマイズは `WithHTTPClient` に `http.RoundTripper`（Transport）を差し替えた `http.Client` を渡して行います
- `Options` 構造体（`ParseDataAlongWithErrors` など）を廃止しました
- レスポンスボディは `data` と `errors` を1パスで読み取ります。HTTP エラーと GraphQL エラーは `NetworkError` / `GqlErrors` を持つエラーとして返ります。gzip 圧縮されたレスポンスにも対応しています
- `graphql.Upload` を variables に含むオペレーションは、`Post` が自動で multipart リクエスト（graphql-multipart-request-spec）を構築します。Upload はエンコード中に検出されるため、v0 と異なりネストした input オブジェクトやリスト内の Upload にも対応します

#### 生成される Go 型の構造変更

GraphQL クエリと Go 型の対応に一貫性を持たせるため、生成規則を次のように変更しました。

1. フラグメント（FragmentSpread）は常に独立した型として生成して利用します
2. フラグメントは構造体に埋め込み（embedded）で配置します
3. フラグメントは公開型として生成します
4. フラグメントは常に non-optional（非ポインタ）です
5. インラインフラグメントは独立した型を生成しません
6. インラインフラグメントは無名構造体として生成します
7. インラインフラグメントは型条件名のフィールドを持つポインタになり、レスポンスの `__typename` が型条件に一致した場合のみ値が入ります（一致しない場合は nil）。判別に `__typename` を使うため、クエリで `__typename` を選択してください
8. クエリのレスポンス型はデフォルトで先頭小文字の非公開型です（`export_query_type: true` で公開型に変更できます）
9. フィールドが optional（ポインタ）かどうかは GraphQL スキーマの NonNull 定義に従います。オブジェクト型のリスト要素は常にポインタです

これに伴い、生成コードも次のように変わりました。

- 構造体タグから `graphql` タグを削除し、`json` タグのみを生成します
- `json` タグに `omitempty` を付与しません。gqlgen の `enable_model_json_omitzero_tag: true` を設定した場合のみ NonNull フィールドに `omitzero` を付与します（デフォルトは false）
- フラグメントの埋め込みとインラインフラグメントのフィールドには `json:"-"` を付与し、生成される `UnmarshalJSONFrom` が同じ JSON データからデコードします
- フラグメントを non-optional の埋め込みにしたことで、Getter 関数の生成量を削減しました

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
type userOperation_User struct {
	User *struct {
		UserFragment1 `json:"-"`
	} `json:"-"`
	UserFragment1 `json:"-"`
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

#### 未使用モデルのフィルタリングを既定動作に変更

- v0 でオプトインだった `onlyUsedModels` 相当の動作を既定にしました。`querygen` / `clientgen` を使わないモデルのみの生成では、クエリで使用されていない Object 型・Enum 型を生成しません（Input 型は常に生成し、Interface は維持します）。Enum 型のフィルタリングは [gqlgo/gqlgenc#309](https://github.com/gqlgo/gqlgenc/pull/309) 相当の変更を含みます
- `querygen` / `clientgen` を使う場合は、レスポンスのデシリアライズに必要なため全ての型を生成します

#### CLI の簡素化

- urfave/cli への依存を削除しました。`generate` / `version` サブコマンドと `--configdir` フラグを廃止し、`gqlgenc` は設定ファイル（`.gqlgenc.yml`）のあるディレクトリで実行します（フラグは `-version` のみ）

### 新機能

#### application/graphql-response+json への対応

- リクエストの `Accept` ヘッダーで `application/graphql-response+json;charset=utf-8` と `application/json;charset=utf-8` を送信し、GraphQL over HTTP 仕様のレスポンス形式に対応しました
- 従来の `application/json` 形式では、非 2xx のボディが GraphQL レスポンスである保証がないため、サーバーはバリデーションエラーなどでも常に 200 OK で返す必要がありました。`application/graphql-response+json` では、仕様準拠のサーバーがパース・バリデーションエラーに 400、認証エラーに 401 など適切な HTTP ステータスコードを返せます
- エラーが HTTP ステータスコードで表現されることで、ロードバランサーや APM など GraphQL を知らない中間レイヤからエラー率を観測でき、リトライやキャッシュ制御もステータスコードベースで正しく動作します（エラーレスポンスが CDN に成功としてキャッシュされる事故も防げます）
- 非 2xx かつ `Content-Type: application/graphql-response+json` のレスポンスは GraphQL サーバー自身が生成した正規のレスポンスであることが保証されるため、プロキシや LB が生成したエラーページと区別してボディを安全にパースできます。`ParseResponse` はステータスコードに関係なくボディのパースを試みるため、仕様準拠サーバーが 400 + `errors` を返した場合は `ErrorResponse` に `NetworkError` と `GqlErrors` の両方が入り、`errors.As` でそれぞれ取り出せます
- `Accept` に両形式を並べることはコンテントネゴシエーションとして機能し、新仕様対応サーバーは `application/graphql-response+json` で、旧来のサーバーは従来どおり `application/json` + 常時 200 で応答します。クライアントの解析処理はどちらの形式でも同じため、サーバー側の対応状況に関係なく動作します

#### 型安全な UnmarshalJSONFrom の生成

- querygen はフラグメントを含む型にのみ `UnmarshalJSONFrom`（json/v2 の `UnmarshalerFrom`）を生成します。通常フィールドはメソッドを持たない別名型（`type plain T`）を経由して json/v2 のデフォルトデコードに任せ、フラグメントスプレッドとインラインフラグメント（`__typename` による型判別）だけを追加でデコードします。フラグメントを含まない型はメソッドを生成せず、デフォルトデコードで処理されます。リフレクションベースのランタイム汎用デコーダ（旧 graphqljson）は不要になりました

#### 型付きオペレーションと Client.Post

- clientgen がオペレーションごとに型付き variables 構造体（`<オペレーション名>Vars`）と `client.Operation[Vars, Res]` 値（`<オペレーション名>Op`）を生成し、`Client.Post` メソッドで実行できます。メソッドの型パラメータには Go 1.27 の generic methods（[golang/go#77273](https://github.com/golang/go/issues/77273)）を使用しています。variables の変数名・型のミスをコンパイル時に検出でき、全オペレーション横断のミドルウェアを `client.Operation` を受けるジェネリック関数として書けます

#### undefined / null の区別（Omittable / omitzero）

- gqlgen の model_gen が生成した `graphql.Omittable[T]` を含む Input 型をそのまま variables として送信でき、未設定（undefined: JSON に含めない）と明示的な null を区別できます（[gqlgo/gqlgenc#269](https://github.com/gqlgo/gqlgenc/issues/269)）
- gqlgen サーバー側も受け取った入力の undefined / null を区別でき（[99designs/gqlgen#3660](https://github.com/99designs/gqlgen/pull/3660)）、Go 1.24 以降の `omitempty` + `IsZero` メソッドによりレスポンスで undefined を返せます（[99designs/gqlgen#3659](https://github.com/99designs/gqlgen/pull/3659)）

#### export_query_type オプション

- ネストしたレスポンス型の型名を公開するか選択できます（デフォルトは先頭小文字の非公開型）

#### @goField ディレクティブへの対応

- スキーマの `@goField(type: "...")` で指定したカスタム Go 型を、クエリレスポンス型のフィールドにも反映します

- エラー型を `ErrorResponse` / `HTTPError` として公開し、`Unwrap` により `errors.As` で GraphQL エラー（`gqlerror.List`）や HTTP エラーを判別できます。GraphQL エラー時も `Client.Post` は部分データを返します。呼び出し単位の `Option` はそのリクエストにのみ適用され、クライアントを変異させません

### 内部改善

- コード生成の内部を「GraphQL オペレーション解析」と「Go 型の構築」（codegen パッケージ）に分離し、テンプレートの行数を大幅に削減しました
- 特定のクエリで発生していた複数の panic を修正しました（[gqlgo/gqlgenc#282](https://github.com/gqlgo/gqlgenc/issues/282)）
- gqlgen で生成した実サーバーに対する統合テストを追加しました（フィールドの Name / Alias、Input の `graphql.Omittable`、union の `__typename` 判別、ネストしたフラグメント、フィールド引数のデフォルト値など）
- テストを testify からテーブル駆動テスト + go-cmp に移行しました
- エラーを `%w` でラップし、原因を辿れるようにしました
- golangci-lint v2（`.golangci.yml`）と GitHub Actions の CI を整備しました

### 未対応の機能

- gqlgen の `struct_fields_always_pointers: true`（gqlgen のデフォルト）には対応しません。`false` を設定してください
- `json` タグの `omitempty`（`enable_model_json_omitempty_tag: true`）には対応しません。`omitzero` を使用してください
- クエリレスポンスでの `graphql.Omittable` には対応しません

### alpha リリース時点の残作業

- README.md の整備
- コード生成テンプレート（template.tmpl）の解説ドキュメント
- コード内コメントの整備
- Example によるテストの参照
