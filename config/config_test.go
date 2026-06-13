package config

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/99designs/gqlgen/codegen/config"
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
			name: "schemaとendpointが両方指定されている場合はエラー",
			args: args{
				file: "testdata/cfg/schema_endpoint.yml",
			},
			want: want{
				err: errors.New("'schema' and 'endpoint' both specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)"),
			},
		},
		{
			name: "schemaとendpointのどちらも指定されていない場合はエラー",
			args: args{
				file: "testdata/cfg/no_source.yml",
			},
			want: want{
				err: errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)"),
			},
		},
		{
			name: "不明なキーが含まれている場合はエラー",
			args: args{
				file: "testdata/cfg/unknownkeys.yml",
			},
			want: want{
				err: errors.New("unable to parse config: [1:1] unknown field \"unknown\"\n>  1 | unknown: foo\n       ^\n   2 | gqlgen:\n   3 |   schema:\n   4 |     - outer"),
			},
		},
		{
			name: "nullable_input_omittableが指定された設定を正しく読み込めることを確認する",
			args: args{
				file: "testdata/cfg/nullable_input_omittable.yml",
			},
			want: want{
				config: &Config{
					GQLGencConfig: &GQLGencConfig{
						Query: []string{"./queries/*.graphql"},
						QueryGen: config.PackageConfig{
							Package: "gen",
						},
						ClientGen: config.PackageConfig{
							Package: "gen",
						},
					},
					GQLGenConfig: &config.Config{
						SchemaFilename: config.StringList{
							"testdata/cfg/glob/bar/bar with spaces.graphql",
							"testdata/cfg/glob/foo/foo.graphql",
						},
						Exec: config.ExecConfig{
							Filename: "generated.go",
						},
						Model: config.PackageConfig{
							Filename: "./gen/models_gen.go",
							Package:  "gen",
						},
						Federation: config.PackageConfig{
							Filename: "generated.go",
						},
						Resolver: config.ResolverConfig{
							Filename: "generated.go",
						},
						NullableInputOmittable: true,
						Directives:             map[string]config.DirectiveConfig{},
						GoInitialisms:          config.GoInitialismsConfig{},
					},
				},
			},
		},
		{
			name: "omitzeroが指定された設定を正しく読み込めることを確認する",
			args: args{
				file: "testdata/cfg/omitzero.yml",
			},
			want: want{
				config: &Config{
					GQLGencConfig: &GQLGencConfig{
						Query: []string{"./queries/*.graphql"},
						QueryGen: config.PackageConfig{
							Package: "gen",
						},
						ClientGen: config.PackageConfig{
							Package: "gen",
						},
					},
					GQLGenConfig: &config.Config{
						SchemaFilename: config.StringList{
							"testdata/cfg/glob/bar/bar with spaces.graphql",
							"testdata/cfg/glob/foo/foo.graphql",
						},
						Exec: config.ExecConfig{
							Filename: "generated.go",
						},
						Model: config.PackageConfig{
							Filename: "./gen/models_gen.go",
							Package:  "gen",
						},
						Federation: config.PackageConfig{
							Filename: "generated.go",
						},
						Resolver: config.ResolverConfig{
							Filename: "generated.go",
						},
						EnableModelJsonOmitzeroTag: new(true),
						Directives:                 map[string]config.DirectiveConfig{},
						GoInitialisms:              config.GoInitialismsConfig{},
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
				err: errors.New("failed to walk schema at root not_walkable/: CreateFile not_walkable/: The system cannot find the file specified."), //nolint:revive // 実際のエラーメッセージと一致させるため
			},
			skipOnGOOS: "!windows",
		},
		{
			name: "存在しないディレクトリを指定した場合はエラー（非Windows）",
			args: args{
				file: "testdata/cfg/unwalkable.yml",
			},
			want: want{
				err: errors.New("failed to walk schema at root not_walkable/: lstat not_walkable/: no such file or directory"),
			},
			skipOnGOOS: "windows",
		},
		{
			// model はサーバー側 (gqlgen) で生成済みのモデルを autobind で使う場合に省略できる
			name: "modelを省略してもquerygenが定義されていれば読み込める",
			args: args{
				file: "testdata/cfg/skip_model.yml",
			},
			want: want{},
		},
		{
			name: "modelもquerygenも未定義の場合はエラー",
			args: args{
				file: "testdata/cfg/no_generator.yml",
			},
			want: want{
				err: errors.New("neither 'model' nor 'querygen' specified, at least one generation target is required"),
			},
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

func TestLoadSchema(t *testing.T) {
	t.Parallel()

	type args struct {
		configFile      string
		responseFile    string
		httpErrorStatus int
		authorization   string
		emptyConfig     bool
	}

	type want struct {
		err           error
		authorization string
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
			name: "リモートスキーマ（introspection）で成功する",
			args: args{
				configFile:   "testdata/cfg/endpoint_only.yml",
				responseFile: "testdata/remote/response_ok.json",
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "不正なリモートスキーマでエラー",
			args: args{
				configFile:   "testdata/cfg/endpoint_only.yml",
				responseFile: "testdata/remote/response_invalid_schema.json",
			},
			want: want{
				err: errors.New("OBJECT Query: must define one or more fields"),
			},
		},
		{
			name: "introspectionクエリがHTTPエラーを返す",
			args: args{
				configFile:      "testdata/cfg/endpoint_only.yml",
				httpErrorStatus: http.StatusInternalServerError,
			},
			want: want{
				err: errors.New("introspect schema failed: introspection query failed"),
			},
		},
		{
			name: "schema.QueryがnullでQuery型を初期化できる",
			args: args{
				configFile:   "testdata/cfg/endpoint_only.yml",
				responseFile: "testdata/remote/response_query_null.json",
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "インターフェース実装を含むスキーマでImplementsソート処理を実行する",
			args: args{
				configFile:   "testdata/cfg/endpoint_only.yml",
				responseFile: "testdata/remote/response_with_implements.json",
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
		{
			// endpoint.headers に設定したヘッダーが introspection リクエストに付与される
			name: "endpointのheadersがintrospectionリクエストに付与される",
			args: args{
				configFile:    "testdata/cfg/endpoint_only.yml",
				responseFile:  "testdata/remote/response_ok.json",
				authorization: "Bearer test-token",
			},
			want: want{
				authorization: "Bearer test-token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg *Config
			var err error
			var mockServer *mockRemoteServer

			switch {
			case tt.args.emptyConfig:
				cfg = &Config{GQLGenConfig: &config.Config{}, GQLGencConfig: &GQLGencConfig{}}
				err = cfg.LoadSchema(t.Context())
			case tt.args.responseFile != "" || tt.args.httpErrorStatus != 0:
				// リモートスキーマのテストケース（mockServerを使用）
				var closeServer func()

				if tt.args.httpErrorStatus != 0 {
					// HTTPエラーをシミュレート
					mockServer, closeServer = newMockRemoteServerWithError(t, tt.args.httpErrorStatus, "Internal Server Error")
				} else {
					// 正常なレスポンスまたはスキーマエラー
					mockServer, closeServer = newMockRemoteServer(t, responseFromFile(tt.args.responseFile))
				}
				defer closeServer()

				// mockServerのURLを使った設定を書き込む
				tmpFile, tmpErr := os.CreateTemp(t.TempDir(), "test-config-*.yml")
				if tmpErr != nil {
					t.Fatalf("Failed to create temp config file: %v", tmpErr)
				}
				defer os.Remove(tmpFile.Name())

				tmpConfig := fmt.Sprintf(`gqlgen:
  model:
    filename: ./gen/models_gen.go
    package: gen
gqlgenc:
  query:
    - "./queries/*.graphql"
  querygen:
    filename: ./gen/query.go
    package: gen
  clientgen:
    filename: ./gen/client.go
    package: gen
  endpoint:
    url: %s
`, mockServer.URL)
				if tt.args.authorization != "" {
					tmpConfig += fmt.Sprintf("    headers:\n      Authorization: %q\n", tt.args.authorization)
				}
				if _, tmpErr := tmpFile.WriteString(tmpConfig); tmpErr != nil {
					t.Fatalf("Failed to write temp config: %v", tmpErr)
				}
				tmpFile.Close()

				cfg, err = LoadConfig(tmpFile.Name())
				if err != nil {
					t.Fatalf("LoadConfig() failed: %v", err)
				}
				err = cfg.LoadSchema(t.Context())
			default:
				// ローカルスキーマのテストケース
				cfg, err = LoadConfig(tt.args.configFile)
				if err != nil {
					t.Fatalf("LoadConfig() failed: %v", err)
				}
				err = cfg.LoadSchema(t.Context())
			}

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

			// introspection リクエストに付与されたヘッダーの検証
			if tt.want.authorization != "" {
				if got := mockServer.header.Get("Authorization"); got != tt.want.authorization {
					t.Errorf("Authorization header = %q, want %q", got, tt.want.authorization)
				}
			}

			// 成功時は基本的な検証
			if tt.want.err == nil {
				if cfg == nil {
					t.Fatal("config = nil, want non-nil")
				}
				if cfg.GQLGenConfig == nil {
					t.Error("GQLGenConfig = nil, want non-nil")
				}
				if cfg.GQLGencConfig == nil {
					t.Error("GQLGencConfig = nil, want non-nil")
				}
				if cfg.GQLGenConfig.Schema == nil {
					t.Error("Schema = nil, want non-nil")
				}
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
			err = cfg.LoadSchema(t.Context())
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

type mockRemoteServer struct {
	URL    string
	body   []byte
	header http.Header
}

//nolint:nonamedreturns // named return "mock" with type "*mockRemoteServer" found
func newMockRemoteServer(t *testing.T, response any) (mock *mockRemoteServer, closeServer func()) {
	t.Helper()

	mock = &mockRemoteServer{}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		mock.header = req.Header.Clone()

		var err error
		mock.body, err = io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}

		var responseBody []byte
		switch v := response.(type) {
		case jsontext.Value:
			responseBody = v
		case responseFromFile:
			responseBody = v.load(t)
		default:
			responseBody, err = json.Marshal(response)
			if err != nil {
				t.Errorf("failed to marshal response: %v", err)
			}
		}

		if _, err = writer.Write(responseBody); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	mock.URL = server.URL

	return mock, func() { server.Close() }
}

type responseFromFile string

func (f responseFromFile) load(t *testing.T) []byte {
	t.Helper()

	content, err := os.ReadFile(string(f))
	if err != nil {
		t.Errorf("failed to read file %s: %v", string(f), err)
	}

	return content
}

//nolint:nonamedreturns // named return "mock" with type "*mockRemoteServer" found
func newMockRemoteServerWithError(t *testing.T, statusCode int, message string) (mock *mockRemoteServer, closeServer func()) {
	t.Helper()

	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(statusCode)
		if _, err := writer.Write([]byte(message)); err != nil {
			t.Errorf("failed to write error response: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	mock = &mockRemoteServer{
		URL: server.URL,
	}

	return mock, func() { server.Close() }
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
