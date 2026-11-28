package main

import (
	"maps"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/Yamashou/gqlgenc/v3/config"
	basicschema "github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/schema"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestLoadSchema(t *testing.T) {
	t.Parallel()

	// introspection に応えるサーバー。スキーマは testdata/integration/basic のもの
	srv := handler.New(basicschema.NewExecutableSchema(basicschema.Config{Resolvers: &basicschema.Resolver{}}))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})
	remote := httptest.NewServer(srv)
	t.Cleanup(remote.Close)

	type fields struct {
		// schemaFiles は、スキーマのファイルの名前と内容。テストごとの一時ディレクトリに書く
		schemaFiles map[string]string
	}

	type args struct {
		// schemaFiles は設定の SchemaFiles。一時ディレクトリからの相対パスで書く
		schemaFiles       []string
		endpoint          *config.EndPointConfig
		federationVersion int
	}

	type want struct {
		// typeNames は、読み込んだスキーマが持つ型のうち、確かめる型の名前
		typeNames []string
		// directiveNames は、読み込んだスキーマが持つディレクティブのうち、確かめるディレクティブの名前
		directiveNames []string
		err            error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "複数のファイルのスキーマを1つにして読み込む",
			fields: fields{
				schemaFiles: map[string]string{
					"query.graphql": "type Query { todos: [Todo!]! }",
					"todo.graphql":  "type Todo { id: ID! }",
				},
			},
			args: args{
				schemaFiles: []string{"query.graphql", "todo.graphql"},
			},
			want: want{
				typeNames: []string{"Query", "Todo"},
			},
		},
		{
			// gqlgen の LoadSchema と同じく、Query 型の無いスキーマには空の Query 型を補う
			name: "Query型が無いスキーマにはQuery型を補う",
			fields: fields{
				schemaFiles: map[string]string{
					"todo.graphql": "type Todo { id: ID! }",
				},
			},
			args: args{
				schemaFiles: []string{"todo.graphql"},
			},
			want: want{
				typeNames: []string{"Query", "Todo"},
			},
		},
		{
			// schema.federation.version を指定すると、@key などの federation のディレクティブを
			// 定義していないスキーマでも読み込める
			name: "federationのバージョンを指定するとfederationのディレクティブを足す",
			fields: fields{
				schemaFiles: map[string]string{
					"todo.graphql": `type Query { todo: Todo } type Todo @key(fields: "id") { id: ID! }`,
				},
			},
			args: args{
				schemaFiles:       []string{"todo.graphql"},
				federationVersion: 2,
			},
			want: want{
				typeNames:      []string{"Query", "Todo"},
				directiveNames: []string{"key"},
			},
		},
		{
			name: "federationのバージョンを指定しなければfederationのディレクティブはエラー",
			fields: fields{
				schemaFiles: map[string]string{
					"todo.graphql": `type Query { todo: Todo } type Todo @key(fields: "id") { id: ID! }`,
				},
			},
			args: args{
				schemaFiles: []string{"todo.graphql"},
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
		{
			name: "スキーマのファイルが無ければエラー",
			args: args{
				schemaFiles: []string{"missing.graphql"},
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
		{
			// schema.endpoint を指定すると、サーバーへ introspection クエリを送ってスキーマを作る
			name: "endpointを指定するとサーバーからintrospectionで取得する",
			args: args{
				endpoint: &config.EndPointConfig{URL: remote.URL},
			},
			want: want{
				typeNames: []string{"Mutation", "Query", "Subscription"},
			},
		},
		{
			name: "endpointのサーバーに届かなければエラー",
			args: args{
				endpoint: &config.EndPointConfig{URL: "http://127.0.0.1:0/graphql"},
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, content := range tt.fields.schemaFiles {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatalf("failed to write %s: %v", name, err)
				}
			}
			cfg := &config.Config{Endpoint: tt.args.endpoint, FederationVersion: tt.args.federationVersion}
			for _, name := range tt.args.schemaFiles {
				cfg.SchemaFiles = append(cfg.SchemaFiles, filepath.Join(dir, name))
			}

			got, err := loadSchema(t.Context(), cfg)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			var gotTypeNames, gotDirectiveNames []string
			if got != nil {
				for _, name := range tt.want.typeNames {
					if _, ok := got.Types[name]; ok {
						gotTypeNames = append(gotTypeNames, name)
					}
				}
				for _, name := range tt.want.directiveNames {
					if _, ok := got.Directives[name]; ok {
						gotDirectiveNames = append(gotDirectiveNames, name)
					}
				}
			}
			if diff := cmp.Diff(tt.want.typeNames, gotTypeNames); diff != "" {
				t.Errorf("typeNames diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.directiveNames, gotDirectiveNames); diff != "" {
				t.Errorf("directiveNames diff(-want +got): %s", diff)
			}
		})
	}
}

func TestBindTypes(t *testing.T) {
	t.Parallel()

	const (
		stringModel = "github.com/99designs/gqlgen/graphql.String"
		timeModel   = "github.com/99designs/gqlgen/graphql.Time"
	)

	// newSchema は、テストごとのスキーマを作る。bindTypes が Implements を並べ替えるため使い回さない
	newSchema := func() *ast.Schema {
		return gqlparser.MustLoadSchema(&ast.Source{
			Name: "schema.graphql",
			Input: `scalar Date
interface Node { id: ID! }
interface Entity { createdAt: Date! }
type User implements Node & Entity { id: ID! createdAt: Date! }
type Query { user: User }`,
		})
	}

	type args struct {
		cfg *config.Config
	}

	type want struct {
		// userImplements は、User が実装する interface の名前
		userImplements []string
		// userModel は、type User の束縛先の Go 型
		userModel []string
		// dateModel は、scalar Date の束縛先の Go 型
		dateModel []string
		// configModels は、bindTypes の後の設定の bind.type.named
		configModels map[string]string
		err          error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// model を生成しない設定では modelgen が動かないため、束縛の無い custom scalar に既定の型を補う
			name: "interfaceを名前順に並べ、束縛の無いcustom scalarをgraphql.Stringへ束縛する",
			args: args{
				cfg: &config.Config{},
			},
			want: want{
				userImplements: []string{"Entity", "Node"},
				dateModel:      []string{stringModel},
				configModels:   nil,
			},
		},
		{
			// gqlgen の Init() は束縛を書き足すが、設定の値は変えない
			name: "bind.type.namedの束縛を使い、設定の値は書き換えない",
			args: args{
				cfg: &config.Config{
					Models: map[string]string{"Date": timeModel},
				},
			},
			want: want{
				userImplements: []string{"Entity", "Node"},
				dateModel:      []string{timeModel},
				configModels:   map[string]string{"Date": timeModel},
			},
		},
		{
			// bind.type.packages のパッケージにある、スキーマの型と同じ名前の Go 型へ束縛する。
			// "パッケージ名.型名" の短縮形の束縛は、import パスを含む形へ直す
			name: "bind.type.packagesのパッケージの同じ名前の型へ束縛し、短縮形の束縛を直す",
			args: args{
				cfg: &config.Config{
					TypeAutobind: []string{"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/domain"},
					Models:       map[string]string{"Date": "domain.Email"},
				},
			},
			want: want{
				userImplements: []string{"Entity", "Node"},
				userModel:      []string{"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/domain.User"},
				dateModel:      []string{"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/domain.Email"},
				configModels:   map[string]string{"Date": "domain.Email"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := newSchema()
			cache := newPackageCache()

			got, err := bindTypes(tt.args.cfg, schema, cache)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			var gotUserImplements, gotUserModel, gotDateModel []string
			if got != nil {
				for _, definition := range got.Schema.Implements["User"] {
					gotUserImplements = append(gotUserImplements, definition.Name)
				}
				gotUserModel = []string(got.Models["User"].Model)
				gotDateModel = []string(got.Models["Date"].Model)

				// 事後条件: 次の設定へ引き継ぐ gqlgen の設定は、今回のものになる
				if cache.gqlgen != got {
					t.Errorf("cache.gqlgen = %p, want %p", cache.gqlgen, got)
				}
			}
			if diff := cmp.Diff(tt.want.userImplements, gotUserImplements); diff != "" {
				t.Errorf("userImplements diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.userModel, gotUserModel); diff != "" {
				t.Errorf("userModel diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.dateModel, gotDateModel); diff != "" {
				t.Errorf("dateModel diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.configModels, tt.args.cfg.Models); diff != "" {
				t.Errorf("configModels diff(-want +got): %s", diff)
			}
		})
	}
}

func TestBindDefaultScalars(t *testing.T) {
	t.Parallel()

	const boundModel = "github.com/example/myapp/domain.Bound"

	type fields struct {
		schema string
		models gqlgenconfig.TypeMap
	}

	type want struct {
		// models は、built-in でない scalar の束縛先の Go 型
		models map[string][]string
	}

	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			// 対応する Go 型が無い custom scalar は graphql.String を既定にする
			name: "対応型が無いcustom scalarはgraphql.Stringを既定にする",
			fields: fields{
				schema: "type Query { f: Unbound }\nscalar Unbound",
				models: gqlgenconfig.TypeMap{},
			},
			want: want{
				models: map[string][]string{"Unbound": {"github.com/99designs/gqlgen/graphql.String"}},
			},
		},
		{
			// autobind / models: で既に束縛済みの scalar はその型を維持し graphql.String で上書きしない
			name: "既に束縛済みのcustom scalarは上書きせずその型を維持する",
			fields: fields{
				schema: "type Query { f: Bound }\nscalar Bound",
				models: gqlgenconfig.TypeMap{"Bound": {Model: gqlgenconfig.StringList{boundModel}}},
			},
			want: want{
				models: map[string][]string{"Bound": {boundModel}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema := gqlparser.MustLoadSchema(&ast.Source{Input: tt.fields.schema})
			bindDefaultScalars(schema, tt.fields.models)

			// 本番では built-in scalar (String など) は injectBuiltins で先に束縛されるが、
			// このテストは bindDefaultScalars を単体で呼ぶため built-in も graphql.String 化される。
			// よって custom scalar だけを確かめる。
			got := make(map[string][]string)
			for _, name := range slices.Sorted(maps.Keys(tt.fields.models)) {
				if definition := schema.Types[name]; definition != nil && !definition.BuiltIn {
					got[name] = []string(tt.fields.models[name].Model)
				}
			}

			if diff := cmp.Diff(tt.want.models, got); diff != "" {
				t.Errorf("models diff(-want +got): %s", diff)
			}
		})
	}
}
