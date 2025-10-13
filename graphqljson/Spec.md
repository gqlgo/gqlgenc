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
