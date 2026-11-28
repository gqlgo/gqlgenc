package config

import (
	"errors"
	"runtime"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/internal/cmputil"
)

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
					SchemaFiles: []string{
						"testdata/cfg/glob/bar/bar with spaces.graphql",
						"testdata/cfg/glob/foo/foo.graphql",
					},
					Query:           []string{"./queries/*.graphql"},
					ModelGen:        config.PackageConfig{Package: "gen"},
					ModelOnlyUsed:   true,
					QueryGen:        config.PackageConfig{Package: "gen"},
					GenerateGetters: true,
					ClientGen:       config.PackageConfig{Package: "gen"},
				},
			},
		},
		{
			// デフォルトは true (クエリで使われる Input / Enum のみ生成)。false を指定すると
			// スキーマの全 Input / Enum 型を生成する
			name: "generate.model.onlyUsed に false を指定した設定を正しく読み込めることを確認する",
			args: args{
				file: "testdata/cfg/model_only_used_false.yml",
			},
			want: want{
				config: &Config{
					SchemaFiles: []string{
						"testdata/cfg/glob/bar/bar with spaces.graphql",
						"testdata/cfg/glob/foo/foo.graphql",
					},
					Query:         []string{"./queries/*.graphql"},
					ModelGen:      config.PackageConfig{Package: "gen"},
					ModelOnlyUsed: false,
					QueryGen:      config.PackageConfig{Package: "gen"},
					ClientGen:     config.PackageConfig{Package: "gen"},
				},
			},
		},
		{
			name: "schema.federation.version を指定した設定を正しく読み込めることを確認する",
			args: args{
				file: "testdata/cfg/federation.yml",
			},
			want: want{
				config: &Config{
					SchemaFiles: []string{
						"testdata/cfg/glob/bar/bar with spaces.graphql",
						"testdata/cfg/glob/foo/foo.graphql",
					},
					FederationVersion: 2,
					Query:             []string{"./queries/*.graphql"},
					ModelGen:          config.PackageConfig{Package: "gen"},
					ModelOnlyUsed:     true,
					QueryGen:          config.PackageConfig{Package: "gen"},
				},
			},
		},
		{
			// endpoint を指定した設定は、スキーマのファイルを持たない
			name: "schema.endpoint を指定した設定を正しく読み込めることを確認する",
			args: args{
				file: "testdata/cfg/endpoint_only.yml",
			},
			want: want{
				config: &Config{
					Endpoint:      &EndPointConfig{URL: "http://mock-server.example.com/graphql"},
					Query:         []string{"./queries/*.graphql"},
					ModelGen:      config.PackageConfig{Package: "gen"},
					ModelOnlyUsed: true,
					QueryGen:      config.PackageConfig{Package: "gen"},
					ClientGen:     config.PackageConfig{Package: "gen"},
				},
			},
		},
		{
			// 境界値: グロブは書いてあるが、一致するファイルが 1 つも無い
			name: "schema.files がどのファイルにも一致しない場合はエラー",
			args: args{
				file: "testdata/cfg/schema_no_match.yml",
			},
			want: want{
				err: errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)"),
			},
		},
		{
			name: "bind.type を指定した設定を正しく読み込めることを確認する",
			args: args{
				file: "testdata/cfg/bind_named.yml",
			},
			want: want{
				config: &Config{
					SchemaFiles: []string{
						"testdata/cfg/glob/bar/bar with spaces.graphql",
						"testdata/cfg/glob/foo/foo.graphql",
					},
					Query:        []string{"./queries/*.graphql"},
					TypeAutobind: []string{"github.com/example/myapp/domain"},
					Models: map[string]string{
						"Email":     "github.com/example/myapp/domain.Email",
						"Timestamp": "github.com/99designs/gqlgen/graphql.Time",
					},
					ModelOnlyUsed: true,
					QueryGen:      config.PackageConfig{Package: "gen"},
				},
			},
		},
		{
			// 境界値: 束縛する型の名前だけを書き、束縛先を書いていない
			name: "bind.type.named の model が無い場合はエラー",
			args: args{
				file: "testdata/cfg/bind_named_without_model.yml",
			},
			want: want{
				err: errors.New("'bind.type.named.Email.model' is required"),
			},
		},
		{
			// bind.type.named に書けるのは model だけ。gqlgen の設定のキー (forceGenerate など) は受け付けない
			name: "bind.type.named に model 以外のキーがある場合はエラー",
			args: args{
				file: "testdata/cfg/bind_named_unknown_key.yml",
			},
			want: want{
				err: errors.New("unable to parse config: [15:9] unknown field \"forceGenerate\"\n  12 |     named:\n  13 |       Email:\n  14 |         model: github.com/example/myapp/domain.Email\n> 15 |         forceGenerate: true\n               ^\n"), //nolint:revive // 実際のエラーメッセージと一致させる
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

			// エラーチェック (メッセージ一致を含む。裸の error は cmp.Comparer が
			// トップレベルで効かないため cmputil ヘルパーを真偽値で使う)
			if !cmputil.EqualErrorMessage(tt.want.err, err) {
				t.Errorf("error = %v, want %v", err, tt.want.err)
			}

			// schemaFilenameのチェック
			if len(tt.want.schemaFilename) > 0 {
				if got == nil {
					t.Error("config = nil, want non-nil")
					return
				}
				if diff := cmp.Diff(tt.want.schemaFilename, got.SchemaFiles); diff != "" {
					t.Errorf("schemaFilename diff(-want +got): %s", diff)
				}
			}

			// configの詳細チェック
			if tt.want.config != nil {
				// 出力先のファイル名は、実行する環境の絶対パスになるため比べない
				opts := []cmp.Option{
					cmpopts.IgnoreFields(config.PackageConfig{}, "Filename"),
				}
				if diff := cmp.Diff(tt.want.config, got, opts...); diff != "" {
					t.Errorf("diff(-want +got): %s", diff)
				}
			}
		})
	}
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
