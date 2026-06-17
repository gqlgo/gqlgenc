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

#### 設定ファイルを gqlgenc 専用の形式に刷新

- gqlgenc が独自の設定スキーマを持ち、クライアント生成に必要な項目だけを公開します。gqlgen の生 config は露出させず、json/v2 タグ・`graphql.Omittable`・`omitzero` など v3 が常に必要とする gqlgen 設定は内部で固定します（`nullable_input_omittable` / `enable_model_json_omitzero_tag` / `struct_fields_always_pointers` などのトグルは廃止）
- トップレベルは `schema`（ソース＋スキーマ設定）/ `query`（クエリソース）/ `bind`（GraphQL → Go 型の解決）/ `generate`（出力ファイル）の4セクションです
- スキーマは `schema.files`（ローカル）か `schema.endpoint`（introspection で取得。URL とヘッダーを指定）のどちらか一方を指定します（同時指定・両方未指定はエラー）
- 生成先は `generate.query.file`（レスポンス型）/ `generate.client.file`（variables・Operation 値）/ `generate.model.file`（input・enum モデル、省略可）で、各ファイルの package はディレクトリ名から導出します
- 既存 Go 型へのバインドは `bind.type.packages`（スキーマ型名）と `bind.fragment.packages`（フラグメント名）に分けます。個別バインドは `bind.type.named`、Federation は `schema.federation.version` です
- nil 安全な Getter の生成は `generate.query.getters: true` で切り替えます。レスポンス型は常に公開型として生成します（旧 `export_query_type` トグルは廃止）
- 設定ファイルに未知のフィールドがあるとエラーになります

```yaml
schema:
  files:
    - ./schema/*.graphql
query:
  files:
    - ./query/*.graphql
generate:
  model:
    file: ./domain/model_gen.go
  query:
    file: ./domain/query_gen.go
  client:
    file: ./query/client_gen.go
```

#### コード生成を querygen と clientgen に分割

- querygen: オペレーションごとのレスポンス型、`UnmarshalJSONFrom`（フラグメントを含む型のみ）、クエリドキュメント定数（`<オペレーション名>Document`）を生成します。nil 安全な Getter は `generate.query.getters: true` のときのみ生成します
- clientgen: 型付き variables 構造体（`<オペレーション名>Vars`）と `client.Operation` 値（`<オペレーション名>Op`）を生成します
- `generate.client.file` を使う場合は `generate.query.file` の指定が必須です。出力先は別パッケージにできます（例: レスポンス型は domain パッケージ、クライアントは query パッケージ）
- 旧 `clientgenv2` / `generator` / `parsequery` / `querydocument` パッケージは `plugins`（modelgen / querygen / clientgen）/ `codegen` / `queryparser` に再編しました
- 生成後のファイルには goimports を適用します

#### ランタイムを client パッケージに刷新（clientv2 廃止）

- `NewClient(client HttpClient, baseURL string, options *Options, interceptors ...RequestInterceptor)` は `NewClient(endpoint string, options ...Option)` になりました
- `RequestInterceptor` と `NewClientWithUnsafeRequestInterceptor` を廃止しました。gqlgenc は `http.Client` の設定責務を持たず、HTTP のカスタマイズ（ヘッダー付与・認証・ロギング・テスト用 transport など）はすべてユーザに委ねます。`WithRoundTripper` / `WithHTTPClient` / `WithHTTPHeader` などのヘルパーは一切提供しません。`Option` は `func(*http.Client) *http.Client` 型なので、使用する `http.Client` を自由に組み立てて返せます（transport を包む Option は自前で書く。RoundTripper はリクエストごとに呼ばれるため動的トークンにも対応でき、`Post` / `Get` / `Subscribe` に渡せばその呼び出しだけに適用されます）
- subscription は query / mutation とは別の `SubscriptionClient`（`NewSubscriptionClient`）に分離しました。`Subscribe` はこの型のメソッドで、`Client` には含まれません。WebSocket URL（`ws://` / `wss://`）を `NewSubscriptionClient` に直接渡します。これに伴い `WithWebSocketEndpoint` と http(s)→ws(s) の変換は廃止しました
- `Options` 構造体（`ParseDataAlongWithErrors` など）を廃止しました
- レスポンスボディは `data` と `errors` を1パスで読み取ります。HTTP エラーと GraphQL エラーは `NetworkError` / `GqlErrors` を持つエラーとして返ります。gzip 圧縮されたレスポンスにも対応しています
- `graphql.Upload` を variables に含むオペレーションは、`Post` が自動で multipart リクエスト（graphql-multipart-request-spec）を構築します。Upload はエンコード中に検出されるため、v0 と異なりネストした input オブジェクトやリスト内の Upload にも対応します。リクエストボディのエンコード中に Upload を検出するため、配列要素や入れ子 input の中の Upload も認識します（[gqlgo/gqlgenc#292](https://github.com/gqlgo/gqlgenc/issues/292)）

#### 生成される Go 型の構造変更

GraphQL クエリと Go 型の対応に一貫性を持たせるため、生成規則を次のように変更しました。

1. フラグメント（FragmentSpread）は常に独立した型として生成して利用します
2. フラグメントは構造体に埋め込み（embedded）で配置します
3. フラグメントは公開型として生成します
4. フラグメントは常に non-optional（非ポインタ）です
5. インラインフラグメントは独立した型を生成しません
6. インラインフラグメントは無名構造体として生成します
7. インラインフラグメントは型条件名のフィールドを持つポインタになり、レスポンスの `__typename` が型条件に一致した場合のみ値が入ります（一致しない場合は nil）。判別に `__typename` を使うため、クエリで `__typename` を選択してください
8. クエリのレスポンス型は公開型として生成します（型名はアンダースコア区切り、例: `GetUser_User`）
9. フィールドが optional（ポインタ）かどうかは GraphQL スキーマの NonNull 定義に従います。オブジェクト型のリスト要素は常にポインタです

これに伴い、生成コードも次のように変わりました。

- 構造体タグから `graphql` タグを削除し、`json` タグのみを生成します
- `json` タグに `omitempty` を付与しません。クエリレスポンス型はデコード専用で `omitzero` がマーシャル時にしか効かないため、レスポンス型のフィールドには `omitzero` を付与しません（入力型・モデルの nullable フィールドには gqlgenc が `omitzero` を常に付与します）
- フラグメントの埋め込みとインラインフラグメントのフィールドには `json:"-"` を付与し、生成される `UnmarshalJSONFrom` が同じ JSON データからデコードします
- フラグメントを non-optional の埋め込みにしたことで、Getter 関数の生成量を削減しました
- レスポンス型の nil セーフ getter は既定で生成しなくなりました。getter は interface 満足には使われず、正常系ではフィールド直接アクセスと等価なため、生成量削減を優先して既定 false にしています。従来どおり getter が必要な場合は `generate.query.getters: true` を指定してください

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
type UserOperation_User struct {
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

#### モデル生成を Input 型と Enum 型に限定

- `generate.model.file` を指定した場合、modelgen は **クエリで使われている Input 型と Enum 型だけ** を生成します。Object / Interface / Union 型は生成しません。クライアントはレスポンスを querygen が生成する専用型へデコードし、再利用したい応答型は `@goFragment` / autobind で既存 Go 型にバインドするため、スキーマの Object 型モデルは不要です。使用判定は変数定義の Input 型（ネストした Input を再帰的に辿る）とセレクションセットの Enum 型から行います。v0 でオプトインだった `onlyUsedModels` 相当で、Enum 型のフィルタリングは [gqlgo/gqlgenc#309](https://github.com/gqlgo/gqlgenc/pull/309) 相当の変更を含みます
- client と server で同じ model パッケージを共有する場合は、`generate.model.file` を指定せず modelgen を動かさないでください（model_gen.go は server 側の gqlgen が生成し、gqlgenc は autobind で参照します）。共有しない場合は `generate.model.file` を指定して Input / Enum を生成します

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
- インラインフラグメント（`... on Type`）を含む選択セットに `__typename` を自動注入します。interface / union のレスポンスは `__typename` で型判別してデコードするため、クエリで `__typename` を明示しなくても正しくデコードされます（注入はパース後・検証前に行い、利用者が手書きした場合と同一の結果になります）
- json/v2 のデフォルトデコードへの移行により、旧 graphqljson が `scalar Map`（`map[string]any`）などの自由形式スカラーをデコードできなかった問題（[gqlgo/gqlgenc#76](https://github.com/gqlgo/gqlgenc/issues/76)）は解決済みです
- `foo_bar` と `fooBar` のように異なるフィールドが同じ Go フィールド名に変換される場合、壊れたコードを生成する代わりに、クエリでの alias 付与を促す明確なエラーを返します（[gqlgo/gqlgenc#108](https://github.com/gqlgo/gqlgenc/issues/108)）
- gqlgen の `@goModel` / `@goEnum` ディレクティブで int ベースの Go enum にバインドする場合（[gqlgo/gqlgenc#229](https://github.com/gqlgo/gqlgenc/issues/229)）、バインド先の型が `json.Marshaler` / `json.Unmarshaler` を実装していれば動作します。GraphQL enum のワイヤー表現は名前の文字列のため、名前と値のマッピングは型側の実装が必要です（実装例: testdata の `domain.Level`）

#### 型付きオペレーションと Client.Post

- clientgen がオペレーションごとに型付き variables 構造体（`<オペレーション名>Vars`）と `client.Operation[Vars, Res]` 値（`<オペレーション名>Op`）を生成し、`Client.Post` メソッドで実行できます。メソッドの型パラメータには Go 1.27 の generic methods（[golang/go#77273](https://github.com/golang/go/issues/77273)）を使用しています。variables の変数名・型のミスをコンパイル時に検出でき、全オペレーション横断のミドルウェアを `client.Operation` を受けるジェネリック関数として書けます

#### Subscription サポート

- `(*Client).Subscribe[Vars, Res]` で subscription を実行できます。[graphql-transport-ws](https://github.com/enisdenjo/graphql-ws) プロトコルで WebSocket 接続し、結果を Go 1.27 の `iter.Seq2[*Res, error]` として逐次返します。`query`/`mutation` と同じ `client.Operation` 値を使えるため、clientgen 側に subscription 固有の生成は不要です（[gqlgo/gqlgenc#32](https://github.com/gqlgo/gqlgenc/issues/32)）
- WebSocket エンドポイントは HTTP エンドポイントの `http(s)` を `ws(s)` に変換して導出します。`client.WithWebSocketEndpoint` で上書きできます

#### HTTP GET と操作種別の型制約

- `(*Client).Get[Vars, Res]` で query オペレーションを HTTP GET として実行できます。[GraphQL-over-HTTP 仕様](https://graphql.github.io/graphql-over-http/draft/)に従い variables を URL に JSON エンコードします。CDN やプロキシでのキャッシュに利用できます
- `client.Operation` に操作種別を表す `Kind` 型パラメータ（`client.Query` / `client.Mutation` / `client.Subscription`）を追加しました。clientgen が各オペレーションの種別を埋め込み、`Get` に mutation を渡す・`Post` に subscription を渡すといった誤用がコンパイルエラーになります（GET で mutation を実行できないという GraphQL 仕様の制約を型で表現します）

#### undefined / null の区別（Omittable / omitzero）

- gqlgen の model_gen が生成した `graphql.Omittable[T]` を含む Input 型をそのまま variables として送信でき、未設定（undefined: JSON に含めない）と明示的な null を区別できます（[gqlgo/gqlgenc#269](https://github.com/gqlgo/gqlgenc/issues/269)）
- フィールドごと・呼び出しごとに undefined / null / 値を使い分けられます。`graphql.Omittable[*string]{}`（省略 = JSON に含めない）、`graphql.OmittableOf[*string](nil)`（明示 null）、`graphql.OmittableOf(&v)`（値）。生成時に `omitzero` を固定する設定は不要で、`graphql.Omittable[T]` により実行時に制御します
- nullable なオペレーション変数（例 `$size: Int`）も clientgen が `graphql.Omittable[*T]` + `omitzero` で生成します。これにより変数を省略（undefined）でき、スキーマのデフォルト（`$x: Int = 5` やフィールド引数の `= ...`）が適用されます。input フィールドと同じ3状態（undefined / null / 値）を扱え、非 nullable（必須）変数は素の型のままです
- gqlgen サーバー側も受け取った入力の undefined / null を区別でき（[99designs/gqlgen#3660](https://github.com/99designs/gqlgen/pull/3660)）、Go 1.24 以降の `omitempty` + `IsZero` メソッドによりレスポンスで undefined を返せます（[99designs/gqlgen#3659](https://github.com/99designs/gqlgen/pull/3659)）

#### generate.query.getters オプション

- `generate.query.getters` オプションを追加しました。query_gen.go の全生成型（レスポンス型・生成フラグメント型）に nil セーフな getter を生成するかを選べます（デフォルト false）

#### @goField ディレクティブへの対応

- スキーマの `@goField(type: "...")` で指定したカスタム Go 型を、クエリレスポンス型のフィールドにも反映します

#### クエリでの @goFragment バインド

- クエリのフラグメント定義やフィールド選択に `@goFragment(type: "import/path.Type")` を付けると、レスポンス型を生成せず指定した既存 Go 型にバインドします。型を共有したり、生成型にできないメソッドを持たせたい場合に使えます。`@goFragment` は gqlgenc が注入するクライアント側のコード生成専用ディレクティブで、サーバーへ送るクエリからは自動的に除去されます
- `bind.fragment.packages` にパッケージを列挙すると、フラグメント名と同名の Go 型がそのパッケージにあれば、`@goFragment` を書かなくても自動でその既存型にバインドします（`bind.type.packages` のクエリ版）。マッチ対象はフラグメント名で、明示的な `@goFragment(type: ...)` が優先されます。サーバーモデル用の `bind.type.packages` とは独立した設定です

- エラー型を `ErrorResponse` / `HTTPError` として公開し、`Unwrap` により `errors.As` で GraphQL エラー（`gqlerror.List`）や HTTP エラーを判別できます。GraphQL エラー時も `Client.Post` は部分データを返します。呼び出し単位の `Option` はそのリクエストにのみ適用され、クライアントを変異させません

- `generate.model.file` を省略するとモデル生成をスキップできます。サーバー側で gqlgen が生成したモデルを `bind.type.packages` で参照する構成に対応します。`generate.model.file` と `generate.query.file` は少なくとも一方の指定が必須です

### 解決済みの issue

v0.x（gqlgo/gqlgenc）で報告されていた以下の issue は v3 で解決しています。詳細は各セクションを参照してください。

- [gqlgo/gqlgenc#32](https://github.com/gqlgo/gqlgenc/issues/32) Subscription Support — `graphql-transport-ws` プロトコルによる WebSocket subscription を `(*Client).Subscribe` として実装しました
- [gqlgo/gqlgenc#46](https://github.com/gqlgo/gqlgenc/issues/46) Generate Getters on Interfaces — モデル側は gqlgen 本体が interface にフィールド getter を生成します。クエリレスポンス側は、インターフェースレベルで選択した共通フィールドをラッパー構造体のフィールドとして直接生成するため、型スイッチなしでアクセスできます
- [gqlgo/gqlgenc#76](https://github.com/gqlgo/gqlgenc/issues/76) `scalar Map`（`map[string]any`）をデコードできない — json/v2 のデフォルトデコードへの移行により解決しました（testdata の `Metadata.properties` に回帰テストあり）
- [gqlgo/gqlgenc#108](https://github.com/gqlgo/gqlgenc/issues/108) `foo_bar` と `fooBar` が同じ Go フィールド名になり重複エラー — 壊れたコードを生成する代わりに、クエリでの alias 付与を促す明確なエラーを返します
- [gqlgo/gqlgenc#229](https://github.com/gqlgo/gqlgenc/issues/229) gqlgen の enum バインド（`@goModel` / `@goEnum`）が動作しない — バインド先の型が `json.Marshaler` / `json.Unmarshaler` を実装していれば動作します（実装例: testdata の `domain.Level`）
- [gqlgo/gqlgenc#292](https://github.com/gqlgo/gqlgenc/issues/292) struct や配列の中にネストした `Upload` が認識されない — multipart リクエストの構築を json/v2 のエンコード中に `graphql.Upload` を収集する方式にしたため、配列要素や入れ子 input の中にある Upload も検出されます
- [gqlgo/gqlgenc#269](https://github.com/gqlgo/gqlgenc/issues/269) undefined と null の区別 — `graphql.Omittable` + `omitzero` への対応で解決しました
- [gqlgo/gqlgenc#282](https://github.com/gqlgo/gqlgenc/issues/282) 特定のクエリで発生する panic — 修正済みです
- [gqlgo/gqlgenc#309](https://github.com/gqlgo/gqlgenc/pull/309) `onlyUsedModels` で enum がフィルタリングされない — 未使用モデルのフィルタリングを既定動作として取り込みました

### 内部改善

- コード生成の内部を「GraphQL オペレーション解析」と「Go 型の構築」（codegen パッケージ）に分離し、テンプレートの行数を大幅に削減しました
- 特定のクエリで発生していた複数の panic を修正しました（[gqlgo/gqlgenc#282](https://github.com/gqlgo/gqlgenc/issues/282)）
- gqlgen で生成した実サーバーに対する統合テストを追加しました（フィールドの Name / Alias、Input の `graphql.Omittable`、union の `__typename` 判別、ネストしたフラグメント、フィールド引数のデフォルト値など）
- テストを testify からテーブル駆動テスト + go-cmp に移行しました
- エラーを `%w` でラップし、原因を辿れるようにしました
- golangci-lint v2（`.golangci.yml`）と GitHub Actions の CI を整備しました
- インラインフラグメントのデコードで `__typename` を再パースせず、デコード済みの `Typename` フィールドから型名を読むように簡素化しました（`__typename` の自動注入で必ずフィールドが存在することを利用）。型名抽出のための JSON 再パースが1回減ります
- クエリドキュメント定数（`<オペレーション名>Document`）をミニファイ（1行・最小空白）して生成するようにしました。リクエストごとに送信されるクエリ文字列のサイズを削減します。GraphQL は空白に意味がなく、文字列リテラルはエスケープされて正規化されるため、ミニファイしても意味は変わりません
- Subscription（WebSocket）のテストを Go 1.27 の `testing/synctest` + `httptest.NewTestServer`（インメモリネットワーク）で書き換え、実ネットワーク接続と実時間待ちを排除して決定的にしました（間欠的な失敗を解消）
- HTTP レスポンス解析（`ParseResponse`）を整理しました。gzip リーダーを `defer` で確実に閉じ、引数の `*http.Response` を破壊的に書き換えないようにし、正常レスポンス時の不要なエラー構造体のアロケーションを削減しました
- ファイルアップロード（multipart）のリクエストボディを `io.Pipe` でストリーミング送信するようにし、ファイル本体をメモリに溜め込まないようにしました。あわせて各ファイルパートの `Content-Type` に `Upload.ContentType` を反映します（未指定時は `application/octet-stream`）
- Subscription の graphql-transport-ws の取り扱いを堅牢にしました。サーバーが `complete` を送らずに WebSocket を正常クローズ（1000 / 1001）した場合をエラーではなく完了として扱い、ハンドシェイク中（`connection_ack` 待ち）に `ping` を受け取っても `pong` で応答して待ち続けます
- オペレーション名の重複チェックが実際には検出していなかった不具合を修正しました。Go 名へ正規化すると衝突するオペレーション（例: `getUser` と `GetUser` がどちらも `GetUser`）を生成時にエラーにします（従来は生成は通り、生成コードのコンパイルエラーになっていました）

### 未対応の機能

- モデルのフィールドを常にポインタにする gqlgen の `struct_fields_always_pointers: true`（gqlgen のデフォルト）には対応しません（gqlgenc は内部で `false` に固定します）
- `json` タグの `omitempty` には対応しません（json/v2 では `omitzero` を使用します）
- クエリレスポンスでの `graphql.Omittable` には対応しません

### alpha リリース時点の残作業

- README.md の整備
- コード生成テンプレート（template.tmpl）の解説ドキュメント
- コード内コメントの整備
- Example によるテストの参照
