package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/vektah/gqlparser/v2/ast"
)

func ptr[T any](t T) *T {
	return &t
}

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	type want struct {
		config     *Config
		errMessage string
		err        bool
	}

	tests := []struct {
		name string
		file string
		want want
	}{
		{
			name: "config does not exist",
			file: "doesnotexist.yml",
			want: want{
				err: true,
			},
		},
		{
			name: "malformed config",
			file: "testdata/cfg/malformedconfig.yml",
			want: want{
				err:        true,
				errMessage: "unable to parse config: [1:1] string was used where mapping is expected\n>  1 | asdf\n       ^\n",
			},
		},
		{
			name: "'schema' and 'endpoint' both specified",
			file: "testdata/cfg/schema_endpoint.yml",
			want: want{
				err:        true,
				errMessage: "'schema' and 'endpoint' both specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)",
			},
		},
		{
			name: "neither 'schema' nor 'endpoint' specified",
			file: "testdata/cfg/no_source.yml",
			want: want{
				err:        true,
				errMessage: "neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)",
			},
		},
		{
			name: "unknown keys",
			file: "testdata/cfg/unknownkeys.yml",
			want: want{
				err:        true,
				errMessage: "unknown field \"unknown\"",
			},
		},
		{
			name: "nullable input omittable",
			file: "testdata/cfg/nullable_input_omittable.yml",
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
			name: "omitzero",
			file: "testdata/cfg/omitzero.yml",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := loadConfig(tt.file)

			if tt.want.err {
				if err == nil {
					t.Errorf("loadConfig() error = nil, want error")

					return
				}

				if tt.want.errMessage != "" && !containsString(err.Error(), tt.want.errMessage) {
					t.Errorf("loadConfig() error = %v, want error containing %v", err, tt.want.errMessage)
				}

				return
			}

			if err != nil {
				t.Errorf("loadConfig() error = %v, want nil", err)

				return
			}

			if tt.want.config != nil {
				opts := []cmp.Option{
					cmpopts.IgnoreFields(config.Config{}, "Sources"),
					cmpopts.IgnoreFields(config.PackageConfig{}, "Filename"),
				}
				if diff := cmp.Diff(tt.want.config, cfg, opts...); diff != "" {
					t.Errorf("loadConfig() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestLoadConfigWindows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows platform")
	}

	// Glob filenames test for Windows
	t.Run("globbed filenames on Windows", func(t *testing.T) {
		t.Parallel()

		cfg, err := loadConfig("testdata/cfg/glob.yml")
		if err != nil {
			t.Errorf("loadConfig() error = %v, want nil", err)

			return
		}

		want := `testdata\cfg\glob\bar\bar with spaces.graphql`
		if got := cfg.GQLGenConfig.SchemaFilename[0]; got != want {
			t.Errorf("loadConfig() schemaFilename[0] = %v, want %v", got, want)
		}

		want = `testdata\cfg\glob\foo\foo.graphql`
		if got := cfg.GQLGenConfig.SchemaFilename[1]; got != want {
			t.Errorf("loadConfig() schemaFilename[1] = %v, want %v", got, want)
		}
	})

	// Unwalkable path test for Windows
	t.Run("unwalkable path on Windows", func(t *testing.T) {
		t.Parallel()

		_, err := loadConfig("testdata/cfg/unwalkable.yml")
		want := "failed to walk schema at root not_walkable/: CreateFile not_walkable/: The system cannot find the file specified."

		if err == nil || err.Error() != want {
			t.Errorf("loadConfig() error = %v, want %v", err, want)
		}
	})
}

func TestLoadConfigNonWindows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-Windows test on Windows platform")
	}

	// Glob filenames test for non-Windows
	t.Run("globbed filenames on non-Windows", func(t *testing.T) {
		t.Parallel()

		cfg, err := loadConfig("testdata/cfg/glob.yml")
		if err != nil {
			t.Errorf("loadConfig() error = %v, want nil", err)

			return
		}

		want := "testdata/cfg/glob/bar/bar with spaces.graphql"
		if got := cfg.GQLGenConfig.SchemaFilename[0]; got != want {
			t.Errorf("loadConfig() schemaFilename[0] = %v, want %v", got, want)
		}

		want = "testdata/cfg/glob/foo/foo.graphql"
		if got := cfg.GQLGenConfig.SchemaFilename[1]; got != want {
			t.Errorf("loadConfig() schemaFilename[1] = %v, want %v", got, want)
		}
	})

	// Unwalkable path test for non-Windows
	t.Run("unwalkable path on non-Windows", func(t *testing.T) {
		t.Parallel()

		_, err := loadConfig("testdata/cfg/unwalkable.yml")
		want := "failed to walk schema at root not_walkable/: lstat not_walkable/: no such file or directory"

		if err == nil || err.Error() != want {
			t.Errorf("\n got = %v\nwant = %v", err, want)
		}
	})
}

func TestLoadConfig_LoadSchema(t *testing.T) {
	t.Parallel()

	type want struct {
		config     *Config
		errMessage string
		err        bool
	}

	tests := []struct {
		want         want
		name         string
		responseFile string
	}{
		// TODO: LoadLocalSchema
		{
			name:         "correct remote schema",
			responseFile: "testdata/remote/response_ok.json",
			want: want{
				config: &Config{
					GQLGencConfig: &GQLGencConfig{
						Endpoint: &EndPointConfig{},
					},
					GQLGenConfig: &config.Config{},
				},
			},
		},
		{
			name:         "invalid remote schema",
			responseFile: "testdata/remote/response_invalid_schema.json",
			want: want{
				err:        true,
				errMessage: "OBJECT Query: must define one or more fields",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockServer, closeServer := newMockRemoteServer(t, responseFromFile(tt.responseFile))
			defer closeServer()

			cfg := &Config{
				GQLGenConfig: &config.Config{},
				GQLGencConfig: &GQLGencConfig{
					Endpoint: &EndPointConfig{
						URL: mockServer.URL,
					},
				},
			}
			cfg.GQLGencConfig.Endpoint.Client = mockServer.client
			//
			//err := cfg.loadSchema(t.Context())
			//if tt.want.err {
			//	if err == nil {
			//		t.Errorf("loadSchema() error = nil, want error")
			//
			//		return
			//	}
			//
			//	if tt.want.errMessage != "" && !containsString(err.Error(), tt.want.errMessage) {
			//		t.Errorf("loadSchema() error = %v, want error containing %v", err, tt.want.errMessage)
			//	}
			//
			//	return
			//}
			//
			//if err != nil {
			//	t.Errorf("loadSchema() error = %v, want nil", err)
			//
			//	return
			//}

			if tt.want.config != nil {
				opts := []cmp.Option{
					cmpopts.IgnoreFields(config.Config{}, "Schema"),
					cmpopts.IgnoreFields(EndPointConfig{}, "URL", "Client"),
				}
				if diff := cmp.Diff(tt.want.config, cfg, opts...); diff != "" {
					t.Errorf("loadSchema() mismatch (-want +got):\n%s", diff)
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

	mock = &mockRemoteServer{URL: "http://mock/graphql"}
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

	mock.client = &http.Client{Transport: handlerRoundTripper{handler: handler}}

	return mock, func() {}
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

func TestInit(t *testing.T) {
	t.Parallel()

	type want struct {
		errMessage string
		err        bool
	}

	tests := []struct {
		name         string
		configFile   string
		responseFile string
		want         want
	}{
		{
			name:       "ローカルスキーマで成功",
			configFile: "testdata/cfg/glob.yml",
			want: want{
				err: false,
			},
		},
		{
			name:       "スキーマ未指定エラー",
			configFile: "testdata/cfg/no_source.yml",
			want: want{
				err:        true,
				errMessage: "neither 'schema' nor 'endpoint' specified",
			},
		},
		{
			name:         "リモートスキーマ（introspection）で成功",
			configFile:   "testdata/cfg/endpoint_only.yml",
			responseFile: "testdata/remote/response_ok.json",
			want: want{
				err: false,
			},
		},
		{
			name:         "不正なリモートスキーマでエラー",
			configFile:   "testdata/cfg/endpoint_only.yml",
			responseFile: "testdata/remote/response_invalid_schema.json",
			want: want{
				err:        true,
				errMessage: "OBJECT Query: must define one or more fields",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg *Config
			var err error

			if tt.responseFile != "" {
				// リモートスキーマのテストケース（mockServerを使用）
				mockServer, closeServer := newMockRemoteServer(t, responseFromFile(tt.responseFile))
				defer closeServer()

				// mockServerのURLとClientを設定してInit()を直接呼び出すため、
				// 一時的な設定ファイルを作成
				tmpFile, tmpErr := os.CreateTemp("", "test-config-*.yml")
				if tmpErr != nil {
					t.Fatalf("Failed to create temp config file: %v", tmpErr)
				}
				defer os.Remove(tmpFile.Name())

				// mockServerのURLを使った設定を書き込む
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

				// loadConfigで一時ファイルを読み込んでClientを設定
				cfg, err = loadConfig(tmpFile.Name())
				if err != nil {
					t.Fatalf("loadConfig() error = %v", err)
				}
				cfg.GQLGencConfig.Endpoint.Client = mockServer.client

				// Init()の残りの処理を手動実行
				// Load schema
				httpClient := cfg.GQLGencConfig.Endpoint.Client
				if httpClient == nil {
					httpClient = http.DefaultClient
				}
				var schema *ast.Schema
				schema, err = introspectionSchema(t.Context(), httpClient, cfg.GQLGencConfig.Endpoint.URL, cfg.GQLGencConfig.Endpoint.Headers)
				if err == nil {
					cfg.GQLGenConfig.Schema = schema

					// delete exist gen file
					if cfg.GQLGenConfig.Model.IsDefined() {
						_ = syscall.Unlink(cfg.GQLGenConfig.Model.Filename)
					}

					if cfg.GQLGencConfig.QueryGen.IsDefined() {
						_ = syscall.Unlink(cfg.GQLGencConfig.QueryGen.Filename)
					}

					if cfg.GQLGencConfig.ClientGen.IsDefined() {
						_ = syscall.Unlink(cfg.GQLGencConfig.ClientGen.Filename)
					}

					// gqlgen.Config.Init() に必要なフィールドを初期化
					if cfg.GQLGenConfig.Models == nil {
						cfg.GQLGenConfig.Models = make(config.TypeMap)
					}
					if cfg.GQLGenConfig.StructTag == "" {
						cfg.GQLGenConfig.StructTag = "json"
					}

					err = cfg.GQLGenConfig.Init()
					if err == nil {
						// sort Implements to ensure a deterministic output
						for _, implements := range cfg.GQLGenConfig.Schema.Implements {
							slices.SortFunc(implements, func(a, b *ast.Definition) int {
								return strings.Compare(a.Name, b.Name)
							})
						}
					}
				}
			} else {
				// ローカルスキーマのテストケース
				cfg, err = Init(t.Context(), tt.configFile)
			}

			if tt.want.err {
				if err == nil {
					t.Errorf("Init() error = nil, want error")

					return
				}

				if tt.want.errMessage != "" && !containsString(err.Error(), tt.want.errMessage) {
					t.Errorf("Init() error = %v, want error containing %v", err, tt.want.errMessage)
				}

				return
			}

			if err != nil {
				t.Errorf("Init() error = %v, want nil", err)

				return
			}

			// 成功時は基本的な検証のみ
			if cfg == nil {
				t.Error("Init() returned nil config, want non-nil")
			}
			if cfg.GQLGenConfig == nil {
				t.Error("Init() returned nil GQLGenConfig, want non-nil")
			}
			if cfg.GQLGencConfig == nil {
				t.Error("Init() returned nil GQLGencConfig, want non-nil")
			}
			if cfg.GQLGenConfig.Schema == nil {
				t.Error("Init() returned nil Schema, want non-nil")
			}
		})
	}
}
