package typebind

import (
	"errors"
	"go/token"
	"go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/go/packages"

	"github.com/Yamashou/gqlgenc/v3/internal/cmputil"
)

func TestBinder_FindType(t *testing.T) {
	t.Parallel()

	// domainPackage は、型 Status だけを持つ束縛先パッケージの型情報を作る
	domainPackage := func() *types.Package {
		pkg := types.NewPackage("example.com/shop/domain", "domain")
		status := types.NewTypeName(token.NoPos, pkg, "Status", nil)
		types.NewNamed(status, types.Typ[types.String], nil)
		pkg.Scope().Insert(status)

		return pkg
	}

	type fields struct {
		pkgs map[string]*packages.Package
	}

	type args struct {
		pkgPath  string
		typeName string
	}

	type want struct {
		goType string
		err    error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "パッケージスコープの型を返す",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": {PkgPath: "example.com/shop/domain", Types: domainPackage()},
				},
			},
			args: args{
				pkgPath:  "example.com/shop/domain",
				typeName: "Status",
			},
			want: want{
				goType: "example.com/shop/domain.Status",
			},
		},
		{
			name: "パッケージに無い型はエラーを返す",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": {PkgPath: "example.com/shop/domain", Types: domainPackage()},
				},
			},
			args: args{
				pkgPath:  "example.com/shop/domain",
				typeName: "OrderStatus",
			},
			want: want{
				err: errors.New("unable to find type: example.com/shop/domain.OrderStatus"),
			},
		},
		{
			// 読み込みエラーのあるパッケージは、型を 1 つも持たない空のパッケージとしてロードされる。
			// 型が無い原因が分かるよう、読み込みのエラーを添える
			name: "読み込みエラーのあるパッケージの型が見つからないときは読み込みエラーを添える",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": {
						PkgPath: "example.com/shop/domain",
						Types:   types.NewPackage("example.com/shop/domain", ""),
						Errors: []packages.Error{
							{Msg: "build constraints exclude all Go files in /src/shop/domain", Kind: packages.ListError},
						},
					},
				},
			},
			args: args{
				pkgPath:  "example.com/shop/domain",
				typeName: "Status",
			},
			want: want{
				err: errors.New("unable to find type: example.com/shop/domain.Status: package example.com/shop/domain was loaded with errors: build constraints exclude all Go files in /src/shop/domain"),
			},
		},
		{
			// 型情報を持たずにロードされたパッケージにも、読み込みのエラーを添える
			name: "型情報の無いパッケージに読み込みエラーがあるときは読み込みエラーを添える",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/order": {
						PkgPath: "example.com/shop/order",
						Errors: []packages.Error{
							{Pos: "order.go:3:1", Msg: "expected declaration, found orders", Kind: packages.ParseError},
						},
					},
				},
			},
			args: args{
				pkgPath:  "example.com/shop/order",
				typeName: "Order",
			},
			want: want{
				err: errors.New("package could not be loaded: example.com/shop/order: package example.com/shop/order was loaded with errors: order.go:3:1: expected declaration, found orders"),
			},
		},
		{
			// Load は、見つからなかった import パスに nil を記録する。添える読み込みエラーは無い
			name: "ロードできずnilを記録したパッケージはロードできないエラーを返す",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/missing": nil,
				},
			},
			args: args{
				pkgPath:  "example.com/shop/missing",
				typeName: "Status",
			},
			want: want{
				err: errors.New("package could not be loaded: example.com/shop/missing"),
			},
		},
		{
			// 境界値: import パスが空の束縛は、組み込みの map / any だけを解決する
			name: "importパスが空のanyは組み込みのanyを返す",
			fields: fields{
				pkgs: map[string]*packages.Package{},
			},
			args: args{
				pkgPath:  "",
				typeName: "any",
			},
			want: want{
				goType: "any",
			},
		},
		{
			name: "importパスが空で組み込みでない型はエラーを返す",
			fields: fields{
				pkgs: map[string]*packages.Package{},
			},
			args: args{
				pkgPath:  "",
				typeName: "Status",
			},
			want: want{
				err: errors.New("package cannot be empty for type: Status"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binder := &Binder{pkgs: tt.fields.pkgs, dirToPath: map[string]string{}}
			got, err := binder.FindType(tt.args.pkgPath, tt.args.typeName)

			if !cmputil.EqualErrorMessage(tt.want.err, err) {
				t.Errorf("error:\nwant: %v\n got: %v", tt.want.err, err)
			}

			goType := ""
			if got != nil {
				goType = got.String()
			}
			if diff := cmp.Diff(tt.want.goType, goType); diff != "" {
				t.Errorf("goType diff(-want +got): %s", diff)
			}
		})
	}
}
