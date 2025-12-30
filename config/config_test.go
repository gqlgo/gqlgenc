package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

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
				err: fmt.Errorf("unable to read config: open doesnotexist.yml: no such file or directory"),
			},
		},
		{
			name: "不正な形式の設定ファイルはエラー",
			args: args{
				file: "testdata/cfg/malformedconfig.yml",
			},
			want: want{
				err: fmt.Errorf("unable to parse config: [1:1] string was used where mapping is expected\n>  1 | asdf\n       ^\n"),
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
				err: fmt.Errorf("unable to parse config: [1:1] unknown field \"unknown\"\n>  1 | unknown: foo\n       ^\n   2 | gqlgen:\n   3 |   schema:\n   4 |     - outer"),
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
						EnableModelJsonOmitzeroTag: ptr(true),
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
				err: fmt.Errorf("failed to walk schema at root not_walkable/: CreateFile not_walkable/: The system cannot find the file specified."),
			},
			skipOnGOOS: "!windows",
		},
		{
			name: "存在しないディレクトリを指定した場合はエラー（非Windows）",
			args: args{
				file: "testdata/cfg/unwalkable.yml",
			},
			want: want{
				err: fmt.Errorf("failed to walk schema at root not_walkable/: lstat not_walkable/: no such file or directory"),
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

func TestLoadSchema(t *testing.T) {
	t.Parallel()

	type args struct {
		configFile      string
		responseFile    string
		httpErrorStatus int
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
				err: fmt.Errorf("OBJECT Query: must define one or more fields"),
			},
		},
		{
			name: "introspectionクエリがHTTPエラーを返す",
			args: args{
				configFile:      "testdata/cfg/endpoint_only.yml",
				httpErrorStatus: http.StatusInternalServerError,
			},
			want: want{
				err: fmt.Errorf("introspect schema failed: introspection query failed"),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg *Config
			var err error

			if tt.args.responseFile != "" || tt.args.httpErrorStatus != 0 {
				// リモートスキーマのテストケース（mockServerを使用）
				var mockServer *mockRemoteServer
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
				tmpFile, tmpErr := os.CreateTemp("", "test-config-*.yml")
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
				if _, tmpErr := tmpFile.WriteString(tmpConfig); tmpErr != nil {
					t.Fatalf("Failed to write temp config: %v", tmpErr)
				}
				tmpFile.Close()

				cfg, err = LoadConfig(tmpFile.Name())
				if err != nil {
					t.Fatalf("LoadConfig() failed: %v", err)
				}
				err = cfg.LoadSchema(t.Context())
			} else {
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

			// 成功時は基本的な検証
			if tt.want.err == nil {
				if cfg == nil {
					t.Error("config = nil, want non-nil")
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
				err: fmt.Errorf("Expected Name, found <EOF>"),
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
				err: fmt.Errorf("Cannot query field"),
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
	client *http.Client
}

//nolint:nonamedreturns // named return "mock" with type "*mockRemoteServer" found
func newMockRemoteServer(t *testing.T, response any) (mock *mockRemoteServer, closeServer func()) {
	t.Helper()

	mock = &mockRemoteServer{}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		var err error
		mock.body, err = io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}

		var responseBody []byte
		switch v := response.(type) {
		case json.RawMessage:
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
	mock.client = server.Client()

	return mock, func() { server.Close() }
}

type handlerRoundTripper struct {
	handler http.Handler
}

func (rt handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	rt.handler.ServeHTTP(recorder, req)
	resp := recorder.Result()
	return resp, nil
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

	handler := http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		writer.WriteHeader(statusCode)
		if _, err := writer.Write([]byte(message)); err != nil {
			t.Errorf("failed to write error response: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	mock = &mockRemoteServer{
		URL:    server.URL,
		client: server.Client(),
	}

	return mock, func() { server.Close() }
}

func ptr[T any](t T) *T {
	return &t
}
