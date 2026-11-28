package gosrc

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// reservation は、Imports.Reserve に渡す import パスとパッケージ名の組。
type reservation struct {
	path string
	name string
}

// newTestImports は、reserved を Reserve し、qualified を Qualifier に順に渡した Imports を返す。
func newTestImports(t *testing.T, destPkgPath string, reserved []reservation, qualified []*types.Package) *Imports {
	t.Helper()

	imports := NewImports(destPkgPath)
	for _, r := range reserved {
		if err := imports.Reserve(r.path, r.name); err != nil {
			t.Fatalf("failed to reserve %s: %v", r.path, err)
		}
	}
	for _, pkg := range qualified {
		imports.Qualifier(pkg)
	}

	return imports
}

// newNamedType は、パッケージ pkg の、基底型が string の名前付きの型を返す。
func newNamedType(pkg *types.Package, name string) *types.Named {
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), types.Typ[types.String], nil)
}

func TestImports_Reserve(t *testing.T) {
	t.Parallel()

	type fields struct {
		destPkgPath string
		// reserved は、先に Reserve してある import
		reserved []reservation
	}

	type args struct {
		path string
		name string
	}

	type want struct {
		specs []*importSpec
		err   error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name:   "パッケージをその名前のimportとして加える",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{path: "encoding/json/v2", name: "json"},
			want: want{
				specs: []*importSpec{{path: "encoding/json/v2", name: "json", alias: "json"}},
			},
		},
		{
			name: "同じパッケージを同じ名前で加えても1つのまま",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				reserved:    []reservation{{path: "example.com/shop/client", name: "client"}},
			},
			args: args{path: "example.com/shop/client", name: "client"},
			want: want{
				specs: []*importSpec{{path: "example.com/shop/client", name: "client", alias: "client"}},
			},
		},
		{
			// 境界値: client と query を同じパッケージに生成するとき
			name:   "生成するファイルと同じパッケージは加えない",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{path: "example.com/shop/gen", name: "gen"},
			want:   want{specs: nil},
		},
		{
			name: "同じパッケージを別の名前で加えるとエラー",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				reserved:    []reservation{{path: "example.com/shop/client", name: "client"}},
			},
			args: args{path: "example.com/shop/client", name: "shopclient"},
			want: want{
				specs: []*importSpec{{path: "example.com/shop/client", name: "client", alias: "client"}},
				err:   cmpopts.AnyError,
			},
		},
		{
			// query の出力先のパッケージ名が client のときなど
			name: "別のパッケージを同じ名前で加えるとエラー",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				reserved:    []reservation{{path: "example.com/shop/client", name: "client"}},
			},
			args: args{path: "example.com/billing/client", name: "client"},
			want: want{
				specs: []*importSpec{{path: "example.com/shop/client", name: "client", alias: "client"}},
				err:   cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			imports := newTestImports(t, tt.fields.destPkgPath, tt.fields.reserved, nil)

			err := imports.Reserve(tt.args.path, tt.args.name)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.specs, imports.specs, cmp.AllowUnexported(importSpec{}), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("specs diff(-want +got): %s", diff)
			}
		})
	}
}

func TestImports_Qualifier(t *testing.T) {
	t.Parallel()

	type fields struct {
		destPkgPath string
		// reserved は、先に Reserve してある import
		reserved []reservation
		// qualified は、先に Qualifier へ渡してあるパッケージ
		qualified []*types.Package
	}

	type args struct {
		pkg *types.Package
	}

	type want struct {
		qualifier string
		specs     []*importSpec
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name:   "初めてのパッケージはimportに加えてパッケージ名を返す",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{pkg: types.NewPackage("example.com/shop/domain", "domain")},
			want: want{
				qualifier: "domain",
				specs:     []*importSpec{{path: "example.com/shop/domain", name: "domain", alias: "domain"}},
			},
		},
		{
			name: "加えてあるパッケージは同じ名前を返す",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				qualified:   []*types.Package{types.NewPackage("example.com/shop/domain", "domain")},
			},
			args: args{pkg: types.NewPackage("example.com/shop/domain", "domain")},
			want: want{
				qualifier: "domain",
				specs:     []*importSpec{{path: "example.com/shop/domain", name: "domain", alias: "domain"}},
			},
		},
		{
			name: "名前の重なる別のパッケージには番号を付けた別名を返す",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				qualified:   []*types.Package{types.NewPackage("example.com/shop/domain", "domain")},
			},
			args: args{pkg: types.NewPackage("example.com/billing/domain", "domain")},
			want: want{
				qualifier: "domain1",
				specs: []*importSpec{
					{path: "example.com/shop/domain", name: "domain", alias: "domain"},
					{path: "example.com/billing/domain", name: "domain", alias: "domain1"},
				},
			},
		},
		{
			name: "名前の重なるパッケージが2つあれば次の番号を付ける",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				qualified: []*types.Package{
					types.NewPackage("example.com/shop/domain", "domain"),
					types.NewPackage("example.com/billing/domain", "domain"),
				},
			},
			args: args{pkg: types.NewPackage("example.com/stock/domain", "domain")},
			want: want{
				qualifier: "domain2",
				specs: []*importSpec{
					{path: "example.com/shop/domain", name: "domain", alias: "domain"},
					{path: "example.com/billing/domain", name: "domain", alias: "domain1"},
					{path: "example.com/stock/domain", name: "domain", alias: "domain2"},
				},
			},
		},
		{
			// 生成するコードが名前を直接書くパッケージ (client など) の名前は変えない
			name: "Reserveした名前と重なるパッケージには別名を返す",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				reserved:    []reservation{{path: "github.com/Yamashou/gqlgenc/v3/client", name: "client"}},
			},
			args: args{pkg: types.NewPackage("example.com/shop/client", "client")},
			want: want{
				qualifier: "client1",
				specs: []*importSpec{
					{path: "github.com/Yamashou/gqlgenc/v3/client", name: "client", alias: "client"},
					{path: "example.com/shop/client", name: "client", alias: "client1"},
				},
			},
		},
		{
			name: "Reserveしたパッケージはその名前を返す",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				reserved:    []reservation{{path: "encoding/json/v2", name: "json"}},
			},
			args: args{pkg: types.NewPackage("encoding/json/v2", "json")},
			want: want{
				qualifier: "json",
				specs:     []*importSpec{{path: "encoding/json/v2", name: "json", alias: "json"}},
			},
		},
		{
			// 境界値: 生成するファイルと同じパッケージの型は、修飾せずに書く
			name:   "生成するファイルと同じパッケージには空文字を返す",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{pkg: types.NewPackage("example.com/shop/gen", "gen")},
			want:   want{qualifier: "", specs: nil},
		},
		{
			name:   "vendorの下のパッケージはvendorより後ろのパスでimportする",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{pkg: types.NewPackage("example.com/shop/vendor/github.com/google/uuid", "uuid")},
			want: want{
				qualifier: "uuid",
				specs:     []*importSpec{{path: "github.com/google/uuid", name: "uuid", alias: "uuid"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			imports := newTestImports(t, tt.fields.destPkgPath, tt.fields.reserved, tt.fields.qualified)

			got := imports.Qualifier(tt.args.pkg)

			if diff := cmp.Diff(tt.want.qualifier, got); diff != "" {
				t.Errorf("qualifier diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.specs, imports.specs, cmp.AllowUnexported(importSpec{}), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("specs diff(-want +got): %s", diff)
			}
		})
	}
}

func TestImports_TypeString(t *testing.T) {
	t.Parallel()

	shopDomain := types.NewPackage("example.com/shop/domain", "domain")
	billingDomain := types.NewPackage("example.com/billing/domain", "domain")
	gen := types.NewPackage("example.com/shop/gen", "gen")

	// メソッドを 1 つ持つ interface
	validator := types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Validate", types.NewSignatureType(nil, nil, nil, nil, nil, false)),
	}, nil).Complete()

	type fields struct {
		destPkgPath string
		// qualified は、先に Qualifier へ渡してあるパッケージ
		qualified []*types.Package
	}

	type args struct {
		typ types.Type
	}

	type want struct {
		typeString string
		specs      []*importSpec
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name:   "組み込みの型はそのまま書く",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{typ: types.NewPointer(types.Typ[types.String])},
			want:   want{typeString: "*string", specs: nil},
		},
		{
			name:   "別のパッケージの型はパッケージ名で修飾しimportに加える",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{typ: types.NewSlice(types.NewPointer(newNamedType(shopDomain, "Status")))},
			want: want{
				typeString: "[]*domain.Status",
				specs:      []*importSpec{{path: "example.com/shop/domain", name: "domain", alias: "domain"}},
			},
		},
		{
			name: "名前の重なるパッケージの型は別名で修飾する",
			fields: fields{
				destPkgPath: "example.com/shop/gen",
				qualified:   []*types.Package{shopDomain},
			},
			args: args{typ: types.NewMap(newNamedType(shopDomain, "Status"), newNamedType(billingDomain, "Invoice"))},
			want: want{
				typeString: "map[domain.Status]domain1.Invoice",
				specs: []*importSpec{
					{path: "example.com/shop/domain", name: "domain", alias: "domain"},
					{path: "example.com/billing/domain", name: "domain", alias: "domain1"},
				},
			},
		},
		{
			// 境界値: 生成するファイルと同じパッケージの型
			name:   "生成するファイルと同じパッケージの型は修飾しない",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{typ: types.NewPointer(newNamedType(gen, "GetUser_User"))},
			want:   want{typeString: "*GetUser_User", specs: nil},
		},
		{
			name:   "空のinterfaceはanyと書く",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{typ: types.NewMap(types.Typ[types.String], types.NewInterfaceType(nil, nil).Complete())},
			want:   want{typeString: "map[string]any", specs: nil},
		},
		{
			name:   "メソッドを持つinterfaceはそのまま書く",
			fields: fields{destPkgPath: "example.com/shop/gen"},
			args:   args{typ: validator},
			want:   want{typeString: "interface{Validate()}", specs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			imports := newTestImports(t, tt.fields.destPkgPath, nil, tt.fields.qualified)

			got := imports.TypeString(tt.args.typ)

			if diff := cmp.Diff(tt.want.typeString, got); diff != "" {
				t.Errorf("typeString diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.specs, imports.specs, cmp.AllowUnexported(importSpec{}), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("specs diff(-want +got): %s", diff)
			}
		})
	}
}
