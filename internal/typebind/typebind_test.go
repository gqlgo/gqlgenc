package typebind

import (
	"errors"
	"go/token"
	"go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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

// newTestPackage は、名前 typeNames の型 (どれも string を基底にする) を持つ束縛先パッケージを作る。
func newTestPackage(t *testing.T, pkgPath, pkgName string, typeNames ...string) *packages.Package {
	t.Helper()

	pkg := types.NewPackage(pkgPath, pkgName)
	for _, typeName := range typeNames {
		obj := types.NewTypeName(token.NoPos, pkg, typeName, nil)
		types.NewNamed(obj, types.Typ[types.String], nil)
		pkg.Scope().Insert(obj)
	}

	return &packages.Package{PkgPath: pkgPath, Name: pkgName, Types: pkg}
}

func TestBinder_AutobindType(t *testing.T) {
	t.Parallel()

	type fields struct {
		pkgs map[string]*packages.Package
	}

	type args struct {
		schemaTypeName string
		autobindPkgs   []string
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
			name: "スキーマの型と同じ名前の型を返す",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
				},
			},
			args: args{
				schemaTypeName: "Status",
				autobindPkgs:   []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "example.com/shop/domain.Status",
			},
		},
		{
			// スキーマの型の名前そのものが無ければ、Go の名前へ直した名前で探す
			name: "Goの名前へ直した名前の型を返す",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "OrderID"),
				},
			},
			args: args{
				schemaTypeName: "order_id",
				autobindPkgs:   []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "example.com/shop/domain.OrderID",
			},
		},
		{
			name: "複数のパッケージにあるときは先に書いたパッケージの型を返す",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
					"example.com/shop/order":  newTestPackage(t, "example.com/shop/order", "order", "Status"),
				},
			},
			args: args{
				schemaTypeName: "Status",
				autobindPkgs:   []string{"example.com/shop/order", "example.com/shop/domain"},
			},
			want: want{
				goType: "example.com/shop/order.Status",
			},
		},
		{
			name: "どのパッケージにも無ければ空文字を返す",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
				},
			},
			args: args{
				schemaTypeName: "Order",
				autobindPkgs:   []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "",
			},
		},
		{
			// 境界値: autobind パッケージの指定が無い
			name: "パッケージの指定が無ければ空文字を返す",
			fields: fields{
				pkgs: map[string]*packages.Package{},
			},
			args: args{
				schemaTypeName: "Status",
				autobindPkgs:   nil,
			},
			want: want{
				goType: "",
			},
		},
		{
			// 読み込みに失敗したパッケージ (nil を覚えている) は、型を探せない
			name: "パッケージを読み込めていなければエラー",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": nil,
				},
			},
			args: args{
				schemaTypeName: "Status",
				autobindPkgs:   []string{"example.com/shop/domain"},
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binder := &Binder{pkgs: tt.fields.pkgs, dirToPath: map[string]string{}}
			got, err := binder.AutobindType(tt.args.schemaTypeName, tt.args.autobindPkgs)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.goType, got); diff != "" {
				t.Errorf("autobind goType diff(-want +got): %s", diff)
			}
		})
	}
}

func TestBinder_QualifyShortBinding(t *testing.T) {
	t.Parallel()

	type fields struct {
		pkgs map[string]*packages.Package
	}

	type args struct {
		binding      string
		autobindPkgs []string
	}

	type want struct {
		goType string
		ok     bool
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "パッケージ名と型名の短縮形をimportパスを含む形にする",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
				},
			},
			args: args{
				binding:      "domain.Status",
				autobindPkgs: []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "example.com/shop/domain.Status",
				ok:     true,
			},
		},
		{
			name: "importパスを含む束縛は短縮形ではない",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
				},
			},
			args: args{
				binding:      "example.com/shop/domain.Status",
				autobindPkgs: []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "",
				ok:     false,
			},
		},
		{
			// 境界値: any のようなパッケージを持たない束縛
			name: "パッケージ名の無い束縛は短縮形ではない",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
				},
			},
			args: args{
				binding:      "any",
				autobindPkgs: []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "",
				ok:     false,
			},
		},
		{
			name: "同じ名前のパッケージがautobindパッケージに無ければfalse",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
				},
			},
			args: args{
				binding:      "order.Status",
				autobindPkgs: []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "",
				ok:     false,
			},
		},
		{
			name: "パッケージに型が無ければfalse",
			fields: fields{
				pkgs: map[string]*packages.Package{
					"example.com/shop/domain": newTestPackage(t, "example.com/shop/domain", "domain", "Status"),
				},
			},
			args: args{
				binding:      "domain.Order",
				autobindPkgs: []string{"example.com/shop/domain"},
			},
			want: want{
				goType: "",
				ok:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binder := &Binder{pkgs: tt.fields.pkgs, dirToPath: map[string]string{}}
			got, ok := binder.QualifyShortBinding(tt.args.binding, tt.args.autobindPkgs)

			if diff := cmp.Diff(tt.want.goType, got); diff != "" {
				t.Errorf("qualified goType diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.ok, ok); diff != "" {
				t.Errorf("ok diff(-want +got): %s", diff)
			}
		})
	}
}
