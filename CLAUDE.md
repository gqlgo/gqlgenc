# UnitTest

## 目的と適用範囲
- domainやlib配下で複雑なロジックを実装する際に、純粋関数としてオンメモリで検証できるテストを書く。
- 外部サービスやストレージとの統合はE2Eテストで担保する。ユニットテストではモックを作成せず、依存が発生しない設計を前提とする。
- `domain` パッケージを追加や変更する際は必ずユニットテストを作成・更新し、振る舞いを担保する。

## ファイルと命名
- テストファイル名は `*_test.go` とし、対象と同一パッケージに配置する
- テスト関数は `Test<対象>` の形式で命名し、1関数につき1つの関心ごとを扱う
- 1つの関数に対するテストは1つのテスト関数にまとめる。同じ関数をテストする複数のテスト関数を作成せず、テーブル駆動テストのテストケースとして追加する (例: `NewDraftOrderNumber` 関数のテストは `TestNewDraftOrderNumber` 関数1つにまとめ、`TestDraftOrderNumber_ValueIncrement` のような別のテスト関数は作成しない)。
- 公開APIに対する表現力を高めるために、テスト名とサブテスト名にはユースケースや期待結果を日本語で記述することを推奨する。
- `domain_test` など別パッケージを新設せず、テスト対象と同じパッケージで定義する。

## テーブル駆動テスト
- テーブル駆動でケースを列挙し、`for _, tt := range tests { t.Run(tt.name, ...) }` で実行する。`args`, `fields` の構造体を用いて入力を明示する。  `want` の構造体を用いて期待値を明示する。
- want構造体持つフィールドは `result` のような抽象的なものは禁止です。具体的な期待する値を表す名前にしてください。(例: `user`, `orders` など)
- 期待値を表す単語は `want` を用い、`expected` は使用しないでください。
- ループ変数は `tt := tt` のように都度コピーは不要です。
- ポインタ値は `tp.Pt` など既存のヘルパーで生成し、Goの組み込みを直接使う場合でも無名関数で値を包んで明示的にする
- `domain` パッケージではテーブル駆動テストとサブテストを組み合わせ、各テスト関数でサブテストを1階層に留める。
- ケース間で共有したい初期値はクロージャやヘルパーで生成し、テストごとの副作用を排除する。
- IDを指定する場合は `organizationID: OrganizationID("1_Organization")` のようにID生成ロジックを経由した表現を用いる。

## アサーションと比較
- 構造体やスライスの比較には `cmp.Diff` を使用し、メッセージは `(-want +got)` 形式で差分を出力する
- `cmp.Diff` の左辺には必ず `want`、右辺には `got` を渡し、差分表示で `-want +got` が維持されるようにする。
- 関数がオブジェクトを返す場合、`type want` 構造体のフィールドの型はオブジェクトの型にする。プリミティブ型のフィールドを個別に定義するのではなく、オブジェクト全体を期待値として扱う。
- 動的に変化するフィールドは `cmpopts.IgnoreFields` や `cmpopts.SortSlices` などで調整する。
- New関数などではIDや時間などテストごとに変化する値は `cmpopts.IgnoreFields` 等で差分比較から除外するか、テスト用の定数を利用する。
- Filter関数のようにIDや時間が変わらないテストでは `cmpopts.IgnoreFields` で、IDや時間を除外してはいけない。
- メソッドでオブジェクトを更新する際には `cmpopts.IgnoreFields` を使わず、オブジェクト全体が同一であることをチェックして不変条件を確認する。
- errorを比較する際には必ず `cmpopts.EquateErrors()` オプションを `cmp.Diff` 関数に渡す。現在のテストでは十分に活用できていないため、対象コードに合わせて積極的に導入する。
- 期待値が任意のエラーである場合は `cmpopts.AnyError` を期待値として設定する。現在のテストでは十分に活用できていないため、対象コードに合わせて積極的に導入する。
- 比較ロジックは、単純な比較も含めて `github.com/google/go-cmp` を基本とし、`stretchr/testify` などの外部アサーションライブラリや独自の比較処理は使用しない。
- 比較結果の出力には `t.Errorf` を使用し、テストを継続させることで複数の失敗を同時に検出する。
- domain.Decimal は Equalメソッドを持つため、`cmp.Diff` で直接比較できる。
- cmp.Diff で比較が難しい場合は 対象の型にEqualメソッドを追加して、比較可能にすることを検討してください。
- errorの文字列を比較したいときは `cmp.Diff(tt.want, err, cmp.Comparer(cmputil.EqualErrorMessage))` で比較をすること。
 
## 可読性とドキュメント性
- ケースごとにコメントを付け、期待される振る舞いやテストデータの意図を残す
- テストデータは実際の業務ドメインに即した名前を用いて、シナリオが一読で理解できるようにする。
- 新しいパターンを導入する場合は、先に既存のテストで同等の問題をどのように解決しているかを調査し、ルールに追記してから採用する。
 
### 境界値と異常系
- 正常系だけでなく、nil、空スライス、0、負数などの境界値を洗い出した上でケース化する
- `domain` 関数のユニットテストでは事前条件を満たす入力を前提としつつ、事後条件を確認できる境界値 (nil、0、負数、長さ0など) を網羅する。
- 事前条件を満たさない入力は極力設計で防ぎつつ、panic や error が仕様として定義されている場合は専用のテストを用意し、`defer` と `recover` で検証する
 
### 並列実行
- 依存がないテストは `t.Parallel()` を利用する。テーブル駆動の場合は外側のテスト関数とサブテスト双方で `t.Parallel()` を呼び、データ競合がないことを確認する
- 並列化できないケース (グローバルな状態を扱う場合など) はコメントで理由を残す。

### コンテキスト
- コンテキストを扱うテストでは `context.Background()` は使わず `t.Context()` を初期値として使用してください。
- `//nolint` コメントを付ける場合は理由を必ず記載する

## ヘルパーの扱い
- 再利用する検証ロジックはプライベートなヘルパー関数に切り出し、`t.Helper()` を呼んで呼び出し元の行番号が出力されるようにする
- ヘルパーはテスト専用の動作に限定し、業務ロジックを内包しない。

## サンプルコード
典型的なテーブル駆動 + サブテスト構成の例。
可能な限り、このサンプルコードに書いている要素以外は追加しないでください。

以下のコマンドで、Goの関数からテストコードの雛形を生成できます。
```shell
$ go tool gotests -w -template_dir=cmd/tool/gotests/templates/ -only <関数名> <Goファイル>
```

```go
package domain

import (
    "errors"
    "testing"

    "github.com/google/go-cmp/cmp"
    "github.com/google/go-cmp/cmp/cmpopts"
)

func TestOrderByIsDisabled(t *testing.T) {
    t.Parallel()

	type fields struct {
		orders          Orders
	}

	type args struct {
		isDisabled   bool
    }
	
    type want struct {
        orders Orders
        err  error
    }

    tests := []struct {
        name string
		fields fields
        args args
        want want
    }{
        {
            name: "IsDisabledがfalseの注文のみを取得できることを確認する",
			fields: fields{
				orders: Orders{
					{ ID: OrderID("1_Order"), OrderNumber: "1001", IsDisabled: false },
                    { ID: OrderID("2_Order"), OrderNumber: "1002", IsDisabled: true },  
				},
			},
			args: args{
				isDisabled: false,
			},
            want: want{
                orders: Orders{
					{ ID: OrderID("1_Order"), OrderNumber: "1001", IsDisabled: false },
                },
            },
        },
		{
			name: "Ordersが空のときはエラー",
			fields: fields{
				orders: Orders{},
			},
			args: args{
				isDisabled: false,
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
            got, err := tt.fields.orders.FilterByIsDisabled(tt.args.isDisabled)

            if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
            }

			// 必要な場合は、cmpopts.IgnoreFields(Order{}, "ID", "CreatedAt", "UpdatedAt") をオプションで渡す。
            if diff := cmp.Diff(tt.want.orders, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
            }
        })
    }
}
```

panicをテストする例

```go
package domain

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestOrderByIsDisabled(t *testing.T) {
	t.Parallel()

	type fields struct {
		orders          Orders
	}

	type args struct {
		isDisabled   bool
	}

	type want struct {
		orders Orders
		err  error
		panic error
	}

	tests := []struct {
		name string
		fields fields
		args args
		want want
	}{
		{
			name: "IsDisabledがfalseの注文のみを取得できることを確認する",
			fields: fields{
				orders: Orders{
					{ ID: OrderID("1_Order"), OrderNumber: "1001", IsDisabled: false },
					{ ID: OrderID("2_Order"), OrderNumber: "1002", IsDisabled: true },
				},
			},
			args: args{
				isDisabled: false,
			},
			want: want{
				orders: Orders{
					{ ID: OrderID("1_Order"), OrderNumber: "1001", IsDisabled: false },
				},
			},
		},
		{
			name: "Ordersが空のときはpanic",
			fields: fields{
				orders: Orders{},
			},
			args: args{
				isDisabled: false,
			},
			want: want{
				panic: cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				err := recover()
				if diff := cmp.Diff(tt.want.panic, err, cmpopts.EquateErrors()); diff != "" {
					t.Errorf("panic diff(-want +got): %s", diff)
				}
			}()
			got, err := tt.fields.orders.FilterByIsDisabled(tt.args.isDisabled)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			// 必要な場合は、cmpopts.IgnoreFields(Order{}, "ID", "CreatedAt", "UpdatedAt") をオプションで渡す。
			if diff := cmp.Diff(tt.want.orders, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
```

