package typebind

import (
	"errors"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/Yamashou/gqlgenc/v3/internal/cmputil"
)

func TestBinder_LoadErrors(t *testing.T) {
	t.Parallel()

	type fields struct {
		binder *Binder
	}

	type want struct {
		err error
	}

	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			name: "読み込みエラーのあるパッケージが無ければnilを返す",
			fields: fields{
				binder: &Binder{pkgs: map[string]*packages.Package{
					"example.com/shop/domain": {PkgPath: "example.com/shop/domain"},
				}},
			},
			want: want{
				err: nil,
			},
		},
		{
			// Load は、見つからなかった import パスに nil を記録する
			name: "ロードできずnilを記録したパッケージは含めない",
			fields: fields{
				binder: &Binder{pkgs: map[string]*packages.Package{
					"example.com/shop/missing": nil,
				}},
			},
			want: want{
				err: nil,
			},
		},
		{
			// 束縛を使わない経路 (binder を持たない呼び出し側) からも呼べる
			name: "binderがnilのときはnilを返す",
			fields: fields{
				binder: nil,
			},
			want: want{
				err: nil,
			},
		},
		{
			// 位置の分かるエラーには位置を付け、分からないエラーはメッセージだけを載せる。
			// エラーの無い product は含めない
			name: "読み込みエラーのあるパッケージをimportパスの昇順にまとめる",
			fields: fields{
				binder: &Binder{pkgs: map[string]*packages.Package{
					"example.com/shop/order": {
						PkgPath: "example.com/shop/order",
						Errors: []packages.Error{
							{Pos: "order.go:12:9", Msg: "undefined: OrderID", Kind: packages.TypeError},
						},
					},
					"example.com/shop/domain": {
						PkgPath: "example.com/shop/domain",
						Errors: []packages.Error{
							{Msg: "build constraints exclude all Go files in /src/shop/domain", Kind: packages.ListError},
						},
					},
					"example.com/shop/product": {PkgPath: "example.com/shop/product"},
				}},
			},
			want: want{
				err: errors.New("package example.com/shop/domain was loaded with errors: build constraints exclude all Go files in /src/shop/domain; package example.com/shop/order was loaded with errors: order.go:12:9: undefined: OrderID"),
			},
		},
		{
			// 境界値: 上限と同じ 3 件はすべて載せ、残りの件数は付けない
			name: "1つのパッケージのエラーが3件のときはすべて載せる",
			fields: fields{
				binder: &Binder{pkgs: map[string]*packages.Package{
					"example.com/shop/order": {
						PkgPath: "example.com/shop/order",
						Errors: []packages.Error{
							{Pos: "order.go:10:2", Msg: "undefined: OrderID", Kind: packages.TypeError},
							{Pos: "order.go:11:2", Msg: "undefined: OrderNumber", Kind: packages.TypeError},
							{Pos: "order.go:12:2", Msg: "undefined: OrderStatus", Kind: packages.TypeError},
						},
					},
				}},
			},
			want: want{
				err: errors.New("package example.com/shop/order was loaded with errors: order.go:10:2: undefined: OrderID, order.go:11:2: undefined: OrderNumber, order.go:12:2: undefined: OrderStatus"),
			},
		},
		{
			name: "1つのパッケージのエラーが3件を超えるときは残りを件数で示す",
			fields: fields{
				binder: &Binder{pkgs: map[string]*packages.Package{
					"example.com/shop/order": {
						PkgPath: "example.com/shop/order",
						Errors: []packages.Error{
							{Pos: "order.go:10:2", Msg: "undefined: OrderID", Kind: packages.TypeError},
							{Pos: "order.go:11:2", Msg: "undefined: OrderNumber", Kind: packages.TypeError},
							{Pos: "order.go:12:2", Msg: "undefined: OrderStatus", Kind: packages.TypeError},
							{Pos: "order.go:13:2", Msg: "undefined: OrderedAt", Kind: packages.TypeError},
							{Pos: "order.go:14:2", Msg: "undefined: CanceledAt", Kind: packages.TypeError},
						},
					},
				}},
			},
			want: want{
				err: errors.New("package example.com/shop/order was loaded with errors: order.go:10:2: undefined: OrderID, order.go:11:2: undefined: OrderNumber, order.go:12:2: undefined: OrderStatus (and 2 more)"),
			},
		},
		{
			// 境界値: 上限と同じ 3 個のパッケージはすべて載せ、残りの個数は付けない
			name: "読み込みエラーのあるパッケージが3個のときはすべて載せる",
			fields: fields{
				binder: &Binder{pkgs: map[string]*packages.Package{
					"example.com/shop/product": {
						PkgPath: "example.com/shop/product",
						Errors:  []packages.Error{{Pos: "product.go:8:2", Msg: "undefined: ProductID", Kind: packages.TypeError}},
					},
					"example.com/shop/order": {
						PkgPath: "example.com/shop/order",
						Errors:  []packages.Error{{Pos: "order.go:10:2", Msg: "undefined: OrderID", Kind: packages.TypeError}},
					},
					"example.com/shop/customer": {
						PkgPath: "example.com/shop/customer",
						Errors:  []packages.Error{{Pos: "customer.go:6:2", Msg: "undefined: CustomerID", Kind: packages.TypeError}},
					},
				}},
			},
			want: want{
				err: errors.New("package example.com/shop/customer was loaded with errors: customer.go:6:2: undefined: CustomerID; package example.com/shop/order was loaded with errors: order.go:10:2: undefined: OrderID; package example.com/shop/product was loaded with errors: product.go:8:2: undefined: ProductID"),
			},
		},
		{
			// 原因が 1 つでも、ロードしたパッケージのすべてがエラーになることがある
			name: "読み込みエラーのあるパッケージが3個を超えるときは残りを個数で示す",
			fields: fields{
				binder: &Binder{pkgs: map[string]*packages.Package{
					"example.com/shop/product": {
						PkgPath: "example.com/shop/product",
						Errors:  []packages.Error{{Pos: "product.go:8:2", Msg: "undefined: ProductID", Kind: packages.TypeError}},
					},
					"example.com/shop/order": {
						PkgPath: "example.com/shop/order",
						Errors:  []packages.Error{{Pos: "order.go:10:2", Msg: "undefined: OrderID", Kind: packages.TypeError}},
					},
					"example.com/shop/customer": {
						PkgPath: "example.com/shop/customer",
						Errors:  []packages.Error{{Pos: "customer.go:6:2", Msg: "undefined: CustomerID", Kind: packages.TypeError}},
					},
					"example.com/shop/shipment": {
						PkgPath: "example.com/shop/shipment",
						Errors:  []packages.Error{{Pos: "shipment.go:9:2", Msg: "undefined: ShipmentID", Kind: packages.TypeError}},
					},
					"example.com/shop/warehouse": {
						PkgPath: "example.com/shop/warehouse",
						Errors:  []packages.Error{{Pos: "warehouse.go:7:2", Msg: "undefined: WarehouseID", Kind: packages.TypeError}},
					},
				}},
			},
			want: want{
				err: errors.New("package example.com/shop/customer was loaded with errors: customer.go:6:2: undefined: CustomerID; package example.com/shop/order was loaded with errors: order.go:10:2: undefined: OrderID; package example.com/shop/product was loaded with errors: product.go:8:2: undefined: ProductID; and 2 more package(s)"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.fields.binder.LoadErrors()

			if !cmputil.EqualErrorMessage(tt.want.err, err) {
				t.Errorf("error:\nwant: %v\n got: %v", tt.want.err, err)
			}
		})
	}
}
