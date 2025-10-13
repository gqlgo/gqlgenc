# 仕様
encoding/json は汎用 JSON 向けの最低限のマッピングを行うライブラリ、graphqljson は GraphQL レスポンス特有の構造、タグ、未知フィールド保持といった文脈を理解して構造体へ落とし込むアダプタ、という違いがあります。
- 前提構造
    標準 encoding/json は「任意の JSON 文書」をそのまま扱いますが、graphqljson は GraphQL レスポンス専用です。data を起点に、クエリで指定したフィールド（エイリアス含む）が階層
    構造で現れることを前提にしています。
- フィールド解決
  encoding/json は json:"name" タグだけを見ます。GraphQL では同じオブジェクトに複数のフラグメントや匿名埋め込み（... on Type）が重なり得るため、graphqljson は
  graphql:"alias:name" タグや匿名埋め込みを解析して複数の構造体フィールドへ値を配布します。
- タグの意味合い
  encoding/json ではタグが「JSON のキー名＝代入先」という単純な関係ですが、graphqljson では GraphQL のフィールド名、エイリアス、JSON タグ（json:"name"）、そして今回追加した
  json:",unknown" のような補助タグを総合的に判断してマッピングします。
- GraphQL 固有の制約
  GraphQL が定義する NonNull/Nullable、インタフェース／フラグメントのような制約を想定したエラー処理や探索ロジックを組み込んでいるのが graphqljson の特徴です。標準 encoding/
  json はこうした前提を持たないため、GraphQL レスポンスをそのままデコードすると必要なフィールドが見つからなかったり、フラグメントを区別できなかったりします。

## encoding/json/jsontext の整理

- 役割は「JSON 構文レイヤ」のストリーム処理。GOEXPERIMENT=jsonv2 を付けて初めてビルドされ、従来の encoding/json とは別にトークン／値のやり取りだけを担当します。
- 主役は Decoder / Encoder。Decoder.ReadToken と Decoder.ReadValue が状態遷移を維持しながらトークン列を検証します。オフセット (InputOffset) を取れるので、エラーメッセージに
  「バイト N」でと出せる仕組みです。
- トークンは Token.Kind() が '{' や '"' など JSON 先頭文字の正規化結果を返し、値全体は Value（[]byte のラッパー）です。Value には Format/Canonicalize などの整形 API、
  AppendFormat 等のユーティリティが揃っています。
- Options（実体は jsonopts.Options）で構文の許容範囲を制御。AllowDuplicateNames、AllowInvalidUTF8、ReorderRawObjects、WithIndent などが、IO レベルの NewDecoder/NewEncoder に
  渡せるパラメータです。
- 生トークン・値を扱うので、今回の graphqljson 実装では jsontext.NewDecoder でレスポンスを舐めて ReadToken でキー、ReadValue で生バイトを抜き出し、フィールド割り当て時にその
  まま再帰処理に渡す構成がしやすくなりました。

## encoding/json/v2 の整理

- こちらは「JSON セマンティックレイヤ」。同じく GOEXPERIMENT=jsonv2 前提で、Marshal/Unmarshal など従来 API と互換の関数群を提供しつつ、挙動は v1 から刷新されています。
- 重要ポイントは Options。DefaultOptionsV2() が安全寄りのデフォルトで、RejectUnknownMembers、MatchCaseInsensitiveNames、StringifyNumbers、FormatNilSliceAsNull などを組み合わ
  せて意味付けを切り換えます。jsontext.Options と合成できるので、構文レイヤとセマンティックレイヤを別々にカスタマイズ可能です。
- Marshaler/Unmarshaler に加え、IO 直接版 (MarshalTo, UnmarshalFrom) や関数型 (MarshalFunc, UnmarshalFunc) も用意され、柔軟に差し込みができます。WithMarshalers/
  WithUnmarshalers オプションで型ごとのハンドラを登録する設計です。
- 構造体タグの仕様が明文化され、case: や omitzero、シングルクォートでのキー指定などが公式ドキュメントに整理されています。デフォルトはケース敏感なフィールドマッチで、
  MatchCaseInsensitiveNames をオンにした場合の副作用もドキュメントされています。
- jsontext と対になるよう、MarshalEncode/UnmarshalDecode が jsontext.Encoder/Decoder を直接受け取れる形になっており、今回のように jsontext.Value を駆使した手書きデコーダと混
  在させることもできます。

今回の実装で活かした点

- jsontext.Decoder の ReadToken/ReadValue、および InputOffset を使うことで、GraphQL 固有のフィールド再分配と厳密な位置情報付きエラーを実現。
- json/v2 に合わせて json.Unmarshaler や encoding.TextUnmarshaler を尊重し、json:",unknown" で保持する生バイトには jsontext.Value をそのままコピーする方針が取れました。

この理解をベースに、graphqljson では構文レイヤ（jsontext）で生データを細かく制御し、必要に応じてセマンティックレイヤ（json/v2）のアンマーシャル機構へ委譲する構成にしてい
ます。
