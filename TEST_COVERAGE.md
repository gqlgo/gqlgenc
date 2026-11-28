# run_test.go テストカバレッジ分析

## テスト構成

### 1. コード生成テスト

#### 検証目的
生成されるGoコードがGraphQLスキーマとクエリ定義から正確に作成され、仕様に準拠していることを確認する

#### 確認内容
- **正常系**:
  - GraphQLスキーマとクエリからクライアントコードを正しく生成
  - 生成されたコードが期待されるファイル内容と完全一致する（run_test.go:256-259）
  - 構造体フィールドの型、タグ、埋め込みが正しい

- **異常系**:
  - 循環フラグメント参照時にエラーが正しく検出される（run_test.go:246-250）
  - コンパイルエラーにならない安全なコードが生成される

### 2. 統合テスト

#### 検証目的
生成されたクライアントコードが実際のGraphQLサーバーと正しく通信し、データのシリアライズ/デシリアライズが正確に動作することを確認する

#### 確認内容
- HTTPリクエスト/レスポンスの正常な送受信（run_test.go:269-273）
- JSON形式でのデータのエンコード/デコード
- GraphQLの変数が正しくリクエストに含まれる
- レスポンスデータが期待される構造体に正しくマッピングされる（run_test.go:284-286）

---

## GraphQL クエリ機能のテスト観点

### データ型のテスト

#### 検証目的
GraphQLの型システムとGoの型システムの間で正確なマッピングが行われ、null安全性が保たれることを確認する

#### プリミティブ型

**確認内容**: 各プリミティブ型が適切なGo型にマッピングされること

| 型 | Go型 | テストケース | 実装箇所 | 確認事項 |
|---|---|---|---|---|
| String | `string` | `Title: "Test Article"` | run_test.go:43 | 文字列が正しく送受信される |
| ID | `string` | `ID: "article-1"` | run_test.go:42 | ID型がstringとして扱われる |
| Float | `float64` | `Rating: 4.5` | run_test.go:53 | 浮動小数点数の精度が保たれる |
| Boolean | `bool` | `Public: true` | run_test.go:72 | 真偽値が正しく変換される |

#### リスト型

**確認内容**: リストの必須性とnull許容性が正しく表現され、Goのスライス型/ポインタ型に適切にマッピングされること

| リスト型の種類 | GraphQL型定義 | Go型 | テストケース | 実装箇所 | 確認事項 |
|---|---|---|---|---|---|
| 必須リスト | `[String!]!` | `[]string` | `Tags: []string{"tag1", "tag2", "tag3"}` | run_test.go:44 | リスト自体もnullではない、要素もnullではない |
| オプショナルリスト | `[String!]` | `*[]string` | `OptionalTags: &[]string{"optional1", "optional2"}` | run_test.go:45 | リスト自体がnullの可能性がある |
| Null許容要素リスト | `[String]!` | `[]*string` | `NullableElementsList: []*string{ptr("element1"), nil, ptr("element2")}` | run_test.go:55-59 | 要素がnullの可能性がある |
| 完全Null許容リスト | `[String]` | `*[]*string` | `FullyNullableList: &[]*string{ptr("nullable1"), nil}` | run_test.go:60-63 | リスト自体も要素もnullの可能性がある |
| ネストしたリスト | `[[String!]!]!` | `[][]string` | `Matrix: [][]string{{"a", "b", "c"}, {"d", "e", "f"}}` | run_test.go:129-132 | 2次元配列が正しく扱われる |

#### Enum型

**確認内容**: Enum値が型安全なGo定数として生成され、バリデーションが機能すること

| Enum | 値 | テストケース | 実装箇所 | 確認事項 |
|---|---|---|---|---|
| Status | ACTIVE, INACTIVE | `Statuses: []domain.Status{domain.StatusActive, domain.StatusInactive}` | run_test.go:64 | Enum値が文字列定数として扱われ、型チェックが機能する |

#### カスタムスカラー型

**確認内容**: カスタムスカラー型が適切なGo型にマッピングされ、シリアライズ/デシリアライズが正しく動作すること

| 型 | Go型 | テストケース | 実装箇所 | 確認事項 |
|---|---|---|---|
| JSON | `*string` | `Data: ptr('{"key":"value","number":123}')` | run_test.go:139 | JSON文字列がそのまま保持される |

#### 複合型

**確認内容**: Union型とInterface型が正しく処理され、型の判別とフィールドアクセスが安全に行われること

| 型 | GraphQL定義 | テストケース | 実装箇所 | 確認事項 |
|---|---|---|---|---|
| Union型 | `Profile (PublicProfile \| PrivateProfile)` | 両方の型のフィールドを持つ構造体 | run_test.go:102-121, 184-192 | 両方の型のフィールドが同時に取得でき、型に応じた値が設定される |
| Interface型 | `Address (PublicAddress, PrivateAddress)` | 両方の実装のフィールドを持つ構造体 | run_test.go:66-93, 173-183 | インターフェースの共通フィールドと各実装固有のフィールドが取得できる |

---

### フラグメント機能のテスト

#### 検証目的
フラグメントの再利用性と型安全性を確保し、コードの重複を排除しながら、GraphQLクエリの構造を保持することを確認する

#### 1. 名前付きフラグメント

**確認内容**:
- フラグメントで定義したフィールドが正しく展開され、構造体に埋め込まれる
- フラグメントのフィールドが他のフィールドと重複しても競合が発生しない
- 生成されたGo構造体でフラグメント型が正しく埋め込まれる（`json:"-"`タグ）

```graphql
fragment UserFragment1 on User { name, profile { ... } }
fragment UserFragment2 on User { name }
```
- **実装箇所**: run_test.go:152-166
- **生成される構造体**:
  ```go
  type UserOperation_User struct {
      UserFragment1 "json:\"-\""  // フラグメントの埋め込み
      UserFragment2 "json:\"-\""
      Name string
  }
  ```

#### 2. インラインフラグメント（型条件）

**確認内容**:
- インラインフラグメントの型条件が正しく評価される
- 型条件に一致する場合のみフィールドが取得される
- ネストしたフラグメント（インライン内で名前付きフラグメントを使用）が正しく展開される

```graphql
... on User { name, ...UserFragment2 }
```
- **実装箇所**: run_test.go:144-151
- **確認事項**: Userフィールド内でインラインフラグメントとネストしたUserFragment2が両方機能する

#### 3. 型条件付き名前付きフラグメント

**確認内容**:
- Union/Interface型の各具象型に対するフラグメントが正しく定義される
- 複数の型条件フラグメントが同じフィールドに適用される
- 各フラグメントのフィールドが構造体に埋め込まれる
- 実際のデータの型に応じて適切なフラグメントのフィールドに値が設定される

```graphql
fragment PublicProfileFields on PublicProfile { id, status }
fragment PrivateProfileFields on PrivateProfile { id, age }
fragment PublicAddressFields on PublicAddress { id, street, public }
fragment PrivateAddressFields on PrivateAddress { id, street, private }
```
- **実装箇所**:
  - Profile (Union型): run_test.go:184-192
  - Address (Interface型): run_test.go:173-183
- **生成される構造体**:
  ```go
  type UserOperation_User_Profile struct {
      PrivateProfileFields "json:\"-\""  // 両方のフラグメントが埋め込まれる
      PublicProfileFields  "json:\"-\""
  }
  ```
- **確認事項**: PrivateProfileの場合はPrivateProfileFieldsに値が設定され、PublicProfileの場合はPublicProfileFieldsに値が設定される

#### 4. 循環フラグメント参照の検出

**確認内容**:
- フラグメントが他のフラグメントを参照する循環が検出される
- コード生成時にエラーが返される
- 無限ループやスタックオーバーフローが発生しない

```graphql
# FragmentA → FragmentB → FragmentA のような循環参照
```
- **実装箇所**: run_test.go:229-231
- **確認事項**: `wantErr: true`で循環参照がエラーとして検出される

---

### フィールドエイリアスのテスト

#### 検証目的
同じフィールドを異なるエイリアスで複数回取得でき、それぞれが独立した構造体フィールドにマッピングされることを確認する

#### 確認内容
| エイリアス種別 | GraphQLクエリ | Go構造体フィールド | テストケース | 実装箇所 | 確認事項 |
|---|---|---|---|---|---|
| 単純なエイリアス | `name2: name` | `Name2 string` | `Name2: "John Doe"` | run_test.go:169 | 同じフィールドを別名で取得できる |
| 異なる引数での複数エイリアス | `smallPic: profilePic(size: 50)`<br>`largePic: profilePic(size: 500)`<br>`defaultPic: profilePic(size: $size)` | `SmallPic string`<br>`LargePic string`<br>`DefaultPic string` | `SmallPic: "https://example.com/pic_1_50.jpg"`<br>`LargePic: "https://example.com/pic_1_500.jpg"`<br>`DefaultPic: "https://example.com/pic_1_100.jpg"` | run_test.go:170-172 | 同じフィールドに異なる引数を渡して複数の値を同時に取得できる |

**重要性**: 異なる引数での複数エイリアスは、画像の複数サイズを一度に取得する場合など、実用的なユースケースで頻繁に使用される

---

### 変数のテスト

#### 検証目的
GraphQLの変数が正しく型チェックされ、Go関数のパラメータとして適切に表現され、リクエスト時に正しくシリアライズされることを確認する

#### 確認内容
| 変数種別 | GraphQL定義 | Go関数シグネチャ | テスト内容 | 実装箇所 | 確認事項 |
|---|---|---|---|---|---|
| 必須変数 | `$articleId: ID!`<br>`$metadataId: ID!` | `articleID string`<br>`metadataID string` | 必須パラメータとして値を渡す | run_test.go:280 | 必須変数は非ポインタ型として生成され、必ず値を渡す必要がある |
| デフォルト値付き変数 | `$size: Int = 100` | `size *int` | 明示的に100を渡してデフォルト値の動作を確認 | run_test.go:277, 280 | デフォルト値がある場合でもポインタ型として生成され、nilまたは値を渡せる |
| オプショナル変数 | `$userId: ID`<br>`$userStatus: Status` | `userID *string`<br>`userStatus *domain.Status` | nilまたは値を渡す | run_test.go:278-280, 291 | オプショナル変数はポインタ型として生成され、nilを渡すことができる |

**重要性**: 変数の型システムがGraphQLとGoの間で正確に対応し、コンパイル時の型安全性が保たれる

---

### フィールド引数のテスト

#### 検証目的
フィールドに渡される引数が正しく処理され、GraphQLリクエストに含まれることを確認する

#### 1. 複数引数

**確認内容**:
- 複数の引数を同時に指定できる
- 各引数が正しく変数にマッピングされる
- 引数の順序が保持される

```graphql
user(id: $userId, status: $userStatus)
```
- **実装箇所**: run_test.go:278-280
- **確認事項**:
  - `userID`と`userStatus`の両方が関数パラメータとして生成される
  - 両方の引数がGraphQLリクエストに正しく含まれる

#### 2. スキーマレベルのデフォルト値

**確認内容**:
- スキーマで定義されたデフォルト値がリゾルバーで使用される
- 引数にnilを渡した場合にデフォルト値が適用される
- 明示的に値を渡した場合はその値が優先される

```graphql
# スキーマ定義
user(id: ID = "default-user-id", status: Status = ACTIVE): User!
```
- **実装箇所**: run_test.go:288-301
- **確認事項**:
  - 引数にnilを渡した場合、リゾルバーでデフォルト値が使われる
  - GraphQL仕様では変数が省略された場合のみデフォルト値が適用されるが、リゾルバーレベルでの対応を検証

**注意**: GraphQLクライアント生成では、デフォルト値があってもポインタ型として生成されるため、nilを明示的に渡す必要がある

---

## GraphQL ミューテーション機能のテスト観点

### Omittable型のテスト（入力値の省略制御）

#### 検証目的
GraphQLの入力フィールドにおける3つの状態（null、undefined、値）を正確に区別し、サーバーに送信できることを確認する

#### GraphQLにおける3つの状態の意味
GraphQLの入力フィールドには以下の3つの状態があり、それぞれ異なる意味を持ちます：

1. **明示的なnull値**: フィールドは送信されるが値はnull
   - 意味: 「このフィールドをnullに設定する」（既存の値をクリアする）
   - JSONリクエスト: `{"name": null}`

2. **省略（undefined）**: フィールド自体が送信されない
   - 意味: 「このフィールドを変更しない」（既存の値を保持する）
   - JSONリクエスト: `{}`（nameフィールドが存在しない）

3. **実際の値**: フィールドに具体的な値が設定される
   - 意味: 「このフィールドを指定した値に更新する」
   - JSONリクエスト: `{"name": "Sam Smith"}`

#### 確認内容
| 状態 | Go実装 | JSONリクエスト | サーバー側の期待値 | 実装箇所 | 確認事項 |
|---|---|---|---|---|---|
| 明示的なnull | `graphql.OmittableOf[*string](nil)` | `{"name": null}` | `"nil"` | run_test.go:305-316 | フィールドがnullとして送信される |
| 省略（undefined） | `graphql.Omittable[*string]{}` | `{}`（nameなし） | `"undefined"` | run_test.go:317-329 | フィールドがリクエストに含まれない |
| 実際の値 | `graphql.OmittableOf[*string](ptr("Sam Smith"))` | `{"name": "Sam Smith"}` | `"Sam Smith"` | run_test.go:330-342 | フィールドに値が設定される |

**重要性**:
- PATCH操作のように「一部のフィールドのみ更新」する場合に必須
- nullと省略を区別できないと、「値をクリアする」と「変更しない」を表現できない
- GraphQL仕様で定義されている動作を正確に実装

**実装のポイント**:
- `graphql.Omittable[T]`型は`ValueOK()`メソッドで3状態を判別
- サーバー側リゾルバーで`ValueOK()`を使って状態を確認

---

### ネストしたInput Object型のテスト

#### 検証目的
複雑なネストした入力オブジェクトが正しく処理され、Omittable型の3状態がネストしたオブジェクトにも適用されることを確認する

#### GraphQL定義
```graphql
input UpdateUserInput {
  id: ID!
  name: String
  settings: UserSettingsInput  # ネストしたInput Object
}

input UserSettingsInput {
  theme: String!
  notifications: Boolean!
}
```

#### 確認内容
| テストケース | Go実装 | JSONリクエスト | 期待値 | 実装箇所 | 確認事項 |
|---|---|---|---|---|---|
| ネストしたオブジェクトに値を設定 | `Settings: graphql.OmittableOf[*domain.UserSettingsInput](&domain.UserSettingsInput{Theme: "dark", Notifications: true})` | `{"settings": {"theme": "dark", "notifications": true}}` | `Theme: "dark"`, `Notifications: true` | run_test.go:344-366 | ネストしたオブジェクトの各フィールドが正しく送信・取得される |
| ネストしたオブジェクトに明示的なnil | `Settings: graphql.OmittableOf[*domain.UserSettingsInput](nil)` | `{"settings": null}` | `Settings: nil` | run_test.go:367-380 | ネストしたオブジェクト全体をnullとして送信できる |
| ネストしたオブジェクトを省略 | `Settings: graphql.Omittable[*domain.UserSettingsInput]{}` | `{}`（settingsなし） | `Settings: nil` | run_test.go:381-394 | ネストしたオブジェクトをリクエストから省略できる |

**重要性**:
- 実際のアプリケーションではInput Objectが複数階層でネストすることが多い
- ネストした各レベルでOmittable型の3状態が正しく機能する必要がある
- 複雑なフォームの部分更新などで必須の機能

**確認事項**:
- ネストしたオブジェクトのシリアライズ/デシリアライズ
- ネストレベルが深くても正しく動作する
- リゾルバーでネストしたオブジェクトの状態を判別できる

---

## テストの網羅性

### ✅ カバーされているGraphQL機能

- [x] プリミティブ型（String, ID, Float, Boolean）
- [x] リスト型（必須、オプショナル、Null許容、ネスト）
- [x] Enum型
- [x] カスタムスカラー型（JSON）
- [x] Union型
- [x] Interface型
- [x] 名前付きフラグメント
- [x] インラインフラグメント
- [x] 型条件付きフラグメント
- [x] 循環フラグメント参照の検出
- [x] フィールドエイリアス
- [x] 同一フィールドへの複数エイリアス（異なる引数）
- [x] 変数（必須、オプショナル、デフォルト値付き）
- [x] フィールドの複数引数
- [x] スキーマレベルのデフォルト値
- [x] ミューテーション
- [x] Omittable型（null、undefined、値の区別）
- [x] ネストしたInput Object型

### ⚠️ 今後追加を検討すべき機能

- [ ] ディレクティブ（@include, @skip, @deprecated）
- [ ] サブスクリプション
- [ ] ファイルアップロード
- [ ] バッチリクエスト
- [ ] エラーハンドリング（GraphQLエラーレスポンス）
- [ ] ページネーション（Connection型）
- [ ] カスタムディレクティブ

---

## テストの品質指標

| 指標 | 値 |
|---|---|
| テストケース数 | 2（basic test, circular fragments test） |
| サブテスト数（basic test内） | 9（Query 2件、Mutation 7件） |
| 期待値のアサーション | 構造体全体の比較（cmp.Diff使用） |
| コード生成の検証 | ファイル内容の完全一致比較 |
| E2Eテスト | GraphQLサーバーを立ち上げて実際にリクエスト送信 |

---

## まとめ

`run_test.go`は、gqlgencのクライアントコード生成機能とGraphQL仕様の主要機能を包括的にテストしています。特に以下の点で優れています：

1. **包括的なデータ型テスト**: プリミティブ型からUnion/Interface型まで幅広くカバー
2. **フラグメント機能の徹底検証**: 名前付き、インライン、型条件、循環参照検出
3. **変数と引数の詳細テスト**: デフォルト値、オプショナル、複数引数
4. **Omittable型の精密な検証**: GraphQLのnull/undefined/値の3状態を正確に区別
5. **E2E統合テスト**: 実際のGraphQLサーバーとの通信を含む完全なテストフロー

このテストスイートにより、gqlgencが生成するクライアントコードがGraphQL仕様に準拠し、実用的なアプリケーション開発で必要となる機能をすべてサポートしていることが保証されています。
