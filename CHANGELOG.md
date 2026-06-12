# Changelog

## v1.0.0-alpha1

v0.x からの全面的な書き直しです。

### 破壊的変更

- モジュールパスを `github.com/Yamashou/gqlgenc` から `github.com/Yamashou/gqlgenc/v3` に変更しました
- Go 1.27 以上が必要です。ビルド・テストには `GOEXPERIMENT=jsonv2` の設定が必要です
- JSON 処理を `encoding/json/v2` / `encoding/json/jsontext` に全面移行しました。独自デコーダの `graphqljson` パッケージは廃止され、生成コードの `UnmarshalJSON` がデコードを担います
- 設定ファイルを `gqlgenc:` / `gqlgen:` の2セクション構成に変更しました。gqlgen 由来の設定（`schema` / `model` / `models` / `autobind` / `federation` など）は `gqlgen:` セクションにそのまま記述します。旧 `generate:` セクションのオプション（`clientInterfaceName` / `onlyUsedModels` など）は廃止されました
- コード生成を `querygen`（レスポンス型・UnmarshalJSON・ドキュメント定数）と `clientgen`（クライアントメソッド）に分割しました。`clientgen` を使う場合は `querygen` の指定が必須です。旧 `clientgenv2` / `generator` / `parsequery` / `querydocument` パッケージは `plugins` / `codegen` / `queryparser` に再編されました
- ランタイムを `clientv2` パッケージから新しい `client` パッケージに置き換えました
  - `NewClient(client HttpClient, baseURL string, options *Options, interceptors ...RequestInterceptor)` は `NewClient(endpoint string, options ...Option)` になりました
  - `RequestInterceptor` と `NewClientWithUnsafeRequestInterceptor` は廃止されました。ヘッダーの付与は `WithHTTPHeader`、それ以外のカスタマイズは `WithHTTPClient` で `http.Client` を差し替えて行います
  - `Options` 構造体（`ParseDataAlongWithErrors` など）は廃止されました
- 未使用モデルのフィルタリングをオプトインから既定動作に変更しました。`querygen` / `clientgen` を使わないモデルのみの生成では、クエリで使用されていない Object 型・Enum 型を生成しません。Enum 型のフィルタリングは [gqlgo/gqlgenc#309](https://github.com/gqlgo/gqlgenc/pull/309) 相当の変更を含みます
- CLI の `generate` / `version` サブコマンドと `--configdir` フラグを削除しました。`gqlgenc` は設定ファイルのあるディレクトリで実行します（フラグは `-version` のみ）

### 新機能

- querygen がオペレーションごとのレスポンス型に型安全な `UnmarshalJSONFrom`（json/v2 の `UnmarshalerFrom`）を生成します。フラグメントを含まない型は `jsontext.Decoder` からトークンを1パスで読んで各フィールドへ直接デコードし、フラグメントスプレッド（`json:"-"` 付きの埋め込み構造体）やインラインフラグメント（`__typename` による型判別）を含む型のみ、値全体を一度だけバッファして同じ JSON データからデコードします。デコードロジックがコンパイル時に確定するため、ランタイムの汎用デコーダが不要になりました
- `export_query_type` オプションを追加しました。ネストしたレスポンス型の型名を公開するか選択できます（デフォルトは先頭小文字の非公開型）
