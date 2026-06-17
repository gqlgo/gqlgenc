package config

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/99designs/gqlgen/codegen/config"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// stubRemoteLoader returns a RemoteSchemaLoader that yields a fixed schema with
// an interface implementation, so LoadSchema's remote path and Implements sort
// can be exercised without a real client/introspection dependency.
func stubRemoteLoader() RemoteSchemaLoader {
	return func(context.Context, string, http.Header) (*ast.Schema, error) {
		return gqlparser.MustLoadSchema(&ast.Source{
			Name: "schema.graphql",
			Input: `interface Node { id: ID! }
type User implements Node { id: ID! }
type Query { user: User }`,
		}), nil
	}
}

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	type args struct {
		file string
	}

	type want struct {
		config         *Config
		schemaFilename []string
		err            error
	}

	tests := []struct {
		name       string
		args       args
		want       want
		skipOnGOOS string // このテストをスキップするOS (例: "windows", "!windows")
	}{
		{
			name: "設定ファイルが存在しない場合はエラー",
			args: args{
				file: "doesnotexist.yml",
			},
			want: want{
				err: errors.New("unable to read config: open doesnotexist.yml: no such file or directory"),
			},
		},
		{
			name: "不正な形式の設定ファイルはエラー",
			args: args{
				file: "testdata/cfg/malformedconfig.yml",
			},
			want: want{
				err: errors.New("unable to parse config: [1:1] string was used where mapping is expected\n>  1 | asdf\n       ^\n"), //nolint:revive // 実際のエラーメッセージと一致させるため
			},
		},
		{
			name: "不明なキーが含まれている場合はエラー",
			args: args{
				file: "testdata/cfg/unknownkeys.yml",
			},
			want: want{
				err: errors.New("unable to parse config: [1:1] unknown field \"unknown\"\n>  1 | unknown: foo\n       ^\n   2 | schema:\n   3 |   files:\n   4 |     - outer\n   5 | "),
			},
		},
		{
			name: "schema.files と schema.endpoint が両方指定されている場合はエラー",
			args: args{
				file: "testdata/cfg/schema_endpoint.yml",
			},
			want: want{
				err: errors.New("'schema.files' and 'schema.endpoint' both specified. Use files to load from local files, use endpoint to load from a remote server (using introspection)"),
			},
		},
		{
			name: "schema も endpoint も指定されていない場合はエラー",
			args: args{
				file: "testdata/cfg/no_source.yml",
			},
			want: want{
				err: errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)"),
			},
		},
		{
			name: "generate.query.file も generate.model.file も未定義の場合はエラー",
			args: args{
				file: "testdata/cfg/no_generator.yml",
			},
			want: want{
				err: errors.New("neither 'generate.query.file' nor 'generate.model.file' specified, at least one generation target is required"),
			},
		},
		{
			name: "generate.client.file を指定して generate.query.file が無い場合はエラー",
			args: args{
				file: "testdata/cfg/client_without_query.yml",
			},
			want: want{
				err: errors.New("'generate.client.file' is set, 'generate.query.file' must be set"),
			},
		},
		{
			// model はサーバー側 (gqlgen) で生成済みのモデルを autobind で使う場合に省略できる
			name: "generate.model.file を省略しても generate.query.file があれば読み込める",
			args: args{
				file: "testdata/cfg/skip_model.yml",
			},
			want: want{},
		},
		{
			name: "generate.query.getters を指定した設定を正しく読み込めることを確認する",
			args: args{
				file: "testdata/cfg/generate_getters.yml",
			},
			want: want{
				config: &Config{
					GQLGencConfig: &GQLGencConfig{
						Query:           []string{"./queries/*.graphql"},
						QueryGen:        config.PackageConfig{Package: "gen"},
						ClientGen:       config.PackageConfig{Package: "gen"},
						GenerateGetters: true,
					},
					GQLGenConfig: &config.Config{
						SchemaFilename: config.StringList{
							"testdata/cfg/glob/bar/bar with spaces.graphql",
							"testdata/cfg/glob/foo/foo.graphql",
						},
						Exec: config.ExecConfig{
							Filename: "generated.go",
						},
						Model: config.PackageConfig{Package: "gen"},
						Resolver: config.ResolverConfig{
							Filename: "generated.go",
						},
						StructTag:                  "json",
						NullableInputOmittable:     true,
						EnableModelJsonOmitzeroTag: new(true),
						Directives:                 map[string]config.DirectiveConfig{},
					},
				},
			},
		},
		{
			name: "globパターンでスキーマファイルを読み込めることを確認する（Windows）",
			args: args{
				file: "testdata/cfg/glob.yml",
			},
			want: want{
				schemaFilename: []string{
					`testdata\cfg\glob\bar\bar with spaces.graphql`,
					`testdata\cfg\glob\foo\foo.graphql`,
				},
			},
			skipOnGOOS: "!windows",
		},
		{
			name: "globパターンでスキーマファイルを読み込めることを確認する（非Windows）",
			args: args{
				file: "testdata/cfg/glob.yml",
			},
			want: want{
				schemaFilename: []string{
					"testdata/cfg/glob/bar/bar with spaces.graphql",
					"testdata/cfg/glob/foo/foo.graphql",
				},
			},
			skipOnGOOS: "windows",
		},
		{
			name: "存在しないディレクトリを指定した場合はエラー（Windows）",
			args: args{
				file: "testdata/cfg/unwalkable.yml",
			},
			want: want{
				err: errors.New(`schema files: walk "not_walkable/": CreateFile not_walkable/: The system cannot find the file specified.`), //nolint:revive // 実際のエラーメッセージと一致させるため
			},
			skipOnGOOS: "!windows",
		},
		{
			name: "存在しないディレクトリを指定した場合はエラー（非Windows）",
			args: args{
				file: "testdata/cfg/unwalkable.yml",
			},
			want: want{
				err: errors.New(`schema files: walk "not_walkable/": lstat not_walkable/: no such file or directory`),
			},
			skipOnGOOS: "windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// skipOnGOOSのチェック
			if tt.skipOnGOOS != "" {
				if tt.skipOnGOOS[0] == '!' {
					// "!windows" の形式: 指定OS以外でスキップ
					if runtime.GOOS != tt.skipOnGOOS[1:] {
						t.Skipf("Skipping test on %s", runtime.GOOS)
					}
				} else {
					// "windows" の形式: 指定OSでスキップ
					if runtime.GOOS == tt.skipOnGOOS {
						t.Skipf("Skipping test on %s", runtime.GOOS)
					}
				}
			}

			got, err := LoadConfig(tt.args.file)

			// エラーチェック
			if tt.want.err != nil {
				if err == nil {
					t.Errorf("error = nil, want error")
					return
				}
				if tt.want.err.Error() != err.Error() {
					t.Errorf("error message = %q, want %q", err.Error(), tt.want.err.Error())
					return
				}
			} else if err != nil {
				t.Errorf("error = %v, want nil", err)
				return
			}

			// schemaFilenameのチェック
			if len(tt.want.schemaFilename) > 0 {
				if got == nil || got.GQLGenConfig == nil {
					t.Error("config or GQLGenConfig = nil, want non-nil")
					return
				}
				if diff := cmp.Diff(tt.want.schemaFilename, []string(got.GQLGenConfig.SchemaFilename)); diff != "" {
					t.Errorf("schemaFilename diff(-want +got): %s", diff)
				}
			}

			// configの詳細チェック
			if tt.want.config != nil {
				opts := []cmp.Option{
					cmpopts.IgnoreFields(config.Config{}, "Sources"),
					cmpopts.IgnoreFields(config.PackageConfig{}, "Filename"),
				}
				if diff := cmp.Diff(tt.want.config, got, opts...); diff != "" {
					t.Errorf("diff(-want +got): %s", diff)
				}
			}
		})
	}
}

func TestLoadConfig_Federation(t *testing.T) {
	t.Parallel()

	cfg, err := LoadConfig("testdata/cfg/federation.yml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// gqlgen.federation.version を指定すると、@key などの federation ディレクティブ定義が
	// スキーマソースに注入される（gqlgen の federation プラグインの InjectSourcesEarly）。
	var injected bool
	for _, src := range cfg.GQLGenConfig.Sources {
		if strings.Contains(src.Input, "directive @key") {
			injected = true
			break
		}
	}
	if !injected {
		t.Error("federation の @key ディレクティブ定義が sources に注入されていない")
	}
}

func TestLoadSchema(t *testing.T) {
	t.Parallel()

	type args struct {
		configFile  string
		emptyConfig bool
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "ローカルスキーマで成功する",
			args: args{
				configFile: "testdata/cfg/glob.yml",
			},
			want: want{
				err: nil,
			},
		},
		{
			// endpoint 指定時は注入された loader がリモートスキーマを返し、
			// LoadSchema が Init と Implements ソートまで行う
			name: "リモート(endpoint)スキーマで成功する",
			args: args{
				configFile: "testdata/cfg/endpoint_only.yml",
			},
			want: want{
				err: nil,
			},
		},
		{
			// schema も endpoint も無い Config は LoadConfig が拒否するため、
			// LoadSchema の防御分岐は直接構築でのみ到達できる
			name: "schemaもendpointも未指定ならエラー",
			args: args{
				emptyConfig: true,
			},
			want: want{
				err: errors.New("neither 'schema' nor 'endpoint' specified"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg *Config
			var err error

			switch {
			case tt.args.emptyConfig:
				cfg = &Config{GQLGenConfig: &config.Config{}, GQLGencConfig: &GQLGencConfig{}}
			default:
				cfg, err = LoadConfig(tt.args.configFile)
				if err != nil {
					t.Fatalf("LoadConfig() failed: %v", err)
				}
			}

			err = cfg.LoadSchema(t.Context(), stubRemoteLoader())

			// エラーチェック
			if tt.want.err != nil {
				if err == nil {
					t.Fatalf("error = nil, want error containing %q", tt.want.err.Error())
				}
				if !containsString(err.Error(), tt.want.err.Error()) {
					t.Errorf("error message = %q, want to contain %q", err.Error(), tt.want.err.Error())
				}

				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}

			// 成功時はスキーマが構築されていることを確認
			if cfg.GQLGenConfig.Schema == nil {
				t.Error("Schema = nil, want non-nil")
			}
		})
	}
}

func TestLoadQuery(t *testing.T) {
	type fields struct {
		query []string
	}

	type args struct {
		configFile string
	}

	type want struct {
		queryDocumentNotNil          bool
		operationQueryDocumentsCount int
		err                          error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "正常なクエリファイルを読み込めることを確認する",
			fields: fields{
				query: []string{"testdata/query/todos.graphql"},
			},
			args: args{
				configFile: "testdata/cfg/glob.yml",
			},
			want: want{
				queryDocumentNotNil:          true,
				operationQueryDocumentsCount: 1,
				err:                          nil,
			},
		},
		{
			name: "複数のクエリファイルを読み込めることを確認する",
			fields: fields{
				query: []string{"testdata/query/todos.graphql", "testdata/query/create_todo.graphql"},
			},
			args: args{
				configFile: "testdata/cfg/glob.yml",
			},
			want: want{
				queryDocumentNotNil:          true,
				operationQueryDocumentsCount: 2,
				err:                          nil,
			},
		},
		{
			name: "空のクエリリストでもエラーにならない",
			fields: fields{
				query: []string{},
			},
			args: args{
				configFile: "testdata/cfg/glob.yml",
			},
			want: want{
				queryDocumentNotNil:          true,
				operationQueryDocumentsCount: 0,
				err:                          nil,
			},
		},
		{
			name: "構文エラーのあるクエリファイルでエラー",
			fields: fields{
				query: []string{"testdata/query/syntax_error.graphql"},
			},
			args: args{
				configFile: "testdata/cfg/glob.yml",
			},
			want: want{
				err: errors.New("Expected Name, found <EOF>"),
			},
		},
		{
			name: "スキーマに存在しないフィールドを参照するクエリでエラー",
			fields: fields{
				query: []string{"testdata/query/invalid_query.graphql"},
			},
			args: args{
				configFile: "testdata/cfg/glob.yml",
			},
			want: want{
				err: errors.New("Cannot query field"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 設定ファイルを読み込む
			cfg, err := LoadConfig(tt.args.configFile)
			if err != nil {
				t.Fatalf("LoadConfig() failed: %v", err)
			}

			// スキーマをロード
			err = cfg.LoadSchema(t.Context(), stubRemoteLoader())
			if err != nil {
				t.Fatalf("LoadSchema() failed: %v", err)
			}

			// テスト用にQuery設定を上書き
			cfg.GQLGencConfig.Query = tt.fields.query

			// LoadQueryを実行
			err = cfg.GQLGencConfig.LoadQuery(cfg.GQLGenConfig.Schema)

			// エラーチェック
			if tt.want.err != nil {
				if err == nil {
					t.Errorf("error = nil, want error")
					return
				}
				if !containsString(err.Error(), tt.want.err.Error()) {
					t.Errorf("error message = %q, want to contain %q", err.Error(), tt.want.err.Error())
					return
				}
			} else if err != nil {
				t.Errorf("error = %v, want nil", err)
				return
			}

			// 成功時の検証
			if tt.want.err == nil {
				if tt.want.queryDocumentNotNil && cfg.GQLGencConfig.QueryDocument == nil {
					t.Error("QueryDocument = nil, want non-nil")
				}
				if got := len(cfg.GQLGencConfig.OperationQueryDocuments); got != tt.want.operationQueryDocumentsCount {
					t.Errorf("OperationQueryDocuments count = %d, want %d", got, tt.want.operationQueryDocumentsCount)
				}
			}
		})
	}
}

// containsString checks if string s contains substring.
func containsString(s, substring string) bool {
	if len(s) < len(substring) || substring == "" {
		return false
	}

	for i := 0; i <= len(s)-len(substring); i++ {
		if s[i:i+len(substring)] == substring {
			return true
		}
	}

	return false
}

func TestHeaderUnmarshalYAML(t *testing.T) {
	t.Parallel()

	type args struct {
		yamlSource string
	}

	type want struct {
		header Header
		err    error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// README に記載しているスカラー形式
			name: "文字列スカラーは単一要素のリストとしてデコードされる",
			args: args{
				yamlSource: `Authorization: "Bearer token"`,
			},
			want: want{
				header: Header{
					"Authorization": []string{"Bearer token"},
				},
			},
		},
		{
			name: "文字列リストはそのままデコードされる",
			args: args{
				yamlSource: `Accept: ["application/json", "text/plain"]`,
			},
			want: want{
				header: Header{
					"Accept": []string{"application/json", "text/plain"},
				},
			},
		},
		{
			name: "文字列でも文字列リストでもない値はエラー",
			args: args{
				yamlSource: `Authorization: 123`,
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got Header
			err := yaml.Unmarshal([]byte(tt.args.yamlSource), &got)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.header, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
