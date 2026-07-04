package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/internal/clientopt"
	"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/domain"
	"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/query"
	"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/schema"
	samepackagegen "github.com/Yamashou/gqlgenc/v3/testdata/integration/samepackage/gen"
)

func Test_IntegrationTest_NoModelGen(t *testing.T) {
	type want struct {
		file          string
		userOperation *domain.UserOperation
	}
	tests := []struct {
		name            string
		testDir         string
		wantErr         bool
		wantErrContains string
		want            want
	}{
		{
			name:    "basic test",
			testDir: "testdata/integration/basic/",
			wantErr: false,
			want: want{
				file: "./want/query_gen.go.txt",
				userOperation: &domain.UserOperation{
					OptionalUser: &domain.UserOperation_OptionalUser{
						Name:  "Sam Smith",
						Email: "sam.smith@example.com",
					},
					Article: &domain.UserOperation_Article{
						ID:           "article-1",
						Title:        "Test Article",
						Tags:         []string{"tag1", "tag2", "tag3"},
						OptionalTags: &[]string{"optional1", "optional2"},
						Comments: []*domain.UserOperation_Article_Comments{
							{ID: "1", Text: "First comment"},
							{ID: "2", Text: "Second comment"},
						},
						OptionalComments: &[]*domain.UserOperation_Article_OptionalComments{
							{ID: "3", Text: "Optional comment"},
						},
						Rating:         4.5,
						OptionalRating: new(3.8),
						NullableElementsList: []*string{
							new("element1"),
							nil,
							new("element2"),
						},
						FullyNullableList: &[]*string{
							new("nullable1"),
							nil,
						},
						Statuses:         []domain.Status{domain.StatusActive, domain.StatusInactive},
						OptionalStatuses: &[]domain.Status{domain.StatusActive},
						Addresses: []*domain.UserOperation_Article_Addresses{
							{
								Street: "Public St",
								AddressView: domain.AddressView{
									ID:     "addr1",
									Street: "Public St",
									Public: true,
								},
								PrivateAddressFields: domain.PrivateAddressFields{
									ID:      "addr1",
									Street:  "Public St",
									Private: false,
								},
							},
							{
								Street: "Private St",
								PrivateAddressFields: domain.PrivateAddressFields{
									ID:      "addr2",
									Street:  "Private St",
									Private: true,
								},
								AddressView: domain.AddressView{
									ID:     "addr2",
									Street: "Private St",
									Public: false,
								},
							},
						},
						OptionalAddresses: &[]*domain.UserOperation_Article_OptionalAddresses{
							{
								Street:        "Optional St",
								PublicAddress: &domain.UserOperation_Article_OptionalAddresses_PublicAddress{Public: false},
								Typename:      "PublicAddress",
							},
						},
						Profiles: []*domain.UserOperation_Article_Profiles{
							{
								PublicProfileFields: domain.PublicProfileFields{
									ID:     "prof1",
									Status: domain.StatusActive,
								},
								PrivateProfileFields: domain.PrivateProfileFields{
									ID: "prof1",
								},
							},
							{
								PrivateProfileFields: domain.PrivateProfileFields{
									ID:  "prof2",
									Age: new(25),
								},
								PublicProfileFields: domain.PublicProfileFields{
									ID: "prof2",
								},
							},
						},
						OptionalProfiles: &[]*domain.UserOperation_Article_OptionalProfiles{
							{
								PublicProfile: &domain.UserOperation_Article_OptionalProfiles_PublicProfile{Status: domain.StatusInactive},
								Typename:      "PublicProfile",
							},
						},
						Matrix: [][]string{
							{"a", "b", "c"},
							{"d", "e", "f"},
						},
						OptionalMatrix: &[][]string{
							{"x", "y"},
						},
						// 入れ子リストの union ([[Profile!]!]!) は [][]* 単一ポインタで生成され __typename で判別される
						ProfileGrid: [][]*domain.UserOperation_Article_ProfileGrid{
							{
								{
									PublicProfile: &domain.UserOperation_Article_ProfileGrid_PublicProfile{ID: "grid1", Status: domain.StatusActive},
									Typename:      "PublicProfile",
								},
								{
									PrivateProfile: &domain.UserOperation_Article_ProfileGrid_PrivateProfile{Age: new(7), ID: "grid2"},
									Typename:       "PrivateProfile",
								},
							},
						},
					},
					Metadata: &domain.UserOperation_Metadata{
						ID:   "metadata-1",
						Data: new(`{"key":"value","number":123}`),
						// scalar Map (map[string]any) のデコード回帰テスト (gqlgo/gqlgenc#76)
						Properties: &map[string]any{
							"propKey1": "123",
							"propKey2": "test",
						},
						// int ベース enum バインド (@goModel/@goEnum) のデコード回帰テスト (gqlgo/gqlgenc#229)
						Level: domain.LevelHigh,
					},
					User: domain.UserOperation_User{
						Email: "john.doe@example.com",
						User: &domain.UserOperation_User_User{
							UserFragment2: domain.UserFragment2{Name: "John Doe"},
							Name:          "John Doe",
						},
						UserFragment1: domain.UserFragment1{
							User: &domain.UserFragment1_User{
								Name: "John Doe",
							},
							Typename: "User",
							Name:     "John Doe",
							Profile: domain.UserFragment1_Profile{
								PrivateProfile: &domain.UserFragment1_Profile_PrivateProfile{
									Age: func() *int { i := 30; return &i }(),
								},
								Typename: "PrivateProfile",
							},
						},
						UserFragment2: domain.UserFragment2{Name: "John Doe"},
						Typename:      "User",
						Name:          "John Doe",
						Name2:         "John Doe",
						SmallPic:      "https://example.com/pic_1_50.jpg",
						LargePic:      "https://example.com/pic_1_500.jpg",
						DefaultPic:    "https://example.com/pic_1_100.jpg",
						Address: domain.UserOperation_User_Address{
							Street: "123 Main St",
							PrivateAddressFields: domain.PrivateAddressFields{
								ID:     "addr1",
								Street: "123 Main St",
							},
							AddressView: domain.AddressView{
								ID:     "addr1",
								Street: "123 Main St",
							},
						},
						Profile: domain.UserOperation_User_Profile{
							PrivateProfileFields: domain.PrivateProfileFields{
								ID:  "profile1",
								Age: func() *int { i := 30; return &i }(),
							},
							PublicProfileFields: domain.PublicProfileFields{
								ID: "profile1",
							},
						},
						Profile2: domain.UserOperation_User_Profile2{
							PrivateProfile: &domain.UserOperation_User_Profile2_PrivateProfile{
								Age: func() *int { i := 30; return &i }(),
							},
							Typename: "PrivateProfile",
						},
						OptionalProfile: &domain.UserOperation_User_OptionalProfile{
							PublicProfileFields: domain.PublicProfileFields{
								ID:     "profile2",
								Status: domain.StatusActive,
							},
							PrivateProfileFields: domain.PrivateProfileFields{
								ID: "profile2",
							},
						},
						OptionalAddress: &domain.UserOperation_User_OptionalAddress{
							Street: "456 Elm St",
							PublicAddress: &domain.UserOperation_User_OptionalAddress_PublicAddress{
								Street: "456 Elm St",
							},
							Typename: "PublicAddress",
						},
					},
				},
			},
		},
		{
			name:    "circular fragments test - should fail due to fragment cycle",
			testDir: "testdata/integration/circular-fragments/",
			wantErr: true,
		},
		{
			// foo_bar と fooBar が同じ Go フィールド名 FooBar に衝突するケース (gqlgo/gqlgenc#108)。
			// model 指定だが Object 型は input/enum のみ生成する filter で model_gen.go から
			// 除去される (model_gen.go は空スタブ) ため、衝突は model_gen.go ではなくクエリ応答型
			// GetObject_Object の生成時に表面化する。
			name:            "duplicate fields test - should fail with a Go field name collision in the query type",
			testDir:         "testdata/integration/duplicate-fields/",
			wantErr:         true,
			wantErrContains: `GetObject_Object map to the same Go field name "FooBar"`,
		},
		{
			// UC1 (generate.model.file 未指定) で enum を autobind/@goModel のいずれにも束縛せず
			// クエリのリーフとして選択すると、panic ではなく型名を含むエラーで失敗する (#4 回帰)。
			// これはリーフ型を解決する CreateGoTypes (GoTypeGenerator) 側の経路。
			name:            "unbound enum test - should fail with a clean error, not panic",
			testDir:         "testdata/integration/unbound-enum/",
			wantErr:         true,
			wantErrContains: `no Go model is bound for GraphQL type "Status"`,
		},
		{
			// UC1 で未束縛の input 型を変数に使うケース。リーフ ok は Boolean (built-in) で
			// 束縛されるため CreateGoTypes は成功し、未束縛は変数 SearchFilter を解決する
			// CreateOperations (OperationGenerator) 側でエラーになる (#4 の OperationGenerator 経路)。
			name:            "unbound input variable test - should fail in the operation generator",
			testDir:         "testdata/integration/unbound-input/",
			wantErr:         true,
			wantErrContains: `failed to create operations: no Go model is bound for GraphQL type "SearchFilter"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic: %v", r)
				}
			}()

			////////////////////////////////////////////////////////////////////////////////////////////////////////////
			// Query and client generation
			t.Chdir(tt.testDir)
			err := run(t.Context())
			if tt.wantErr {
				switch {
				case err == nil:
					t.Errorf("run() expected error but got nil")
				case tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains):
					t.Errorf("run() error = %q, want it to contain %q", err, tt.wantErrContains)
				}
				return // エラーが期待される場合はここでテストを終了
			}
			if err != nil {
				t.Errorf("run() error = %v", err)
			}

			// Compare the content of the generated file with the want file
			actualFilePath := "domain/query_gen.go"
			wantFilePath := tt.want.file
			compareFiles(t, wantFilePath, actualFilePath)

			////////////////////////////////////////////////////////////////////////////////////////////////////////////
			// send request test
			ctx := t.Context()

			es := schema.NewExecutableSchema(schema.Config{Resolvers: &schema.Resolver{}})
			srv := handler.New(es)
			srv.AddTransport(transport.POST{})
			srv.AddTransport(transport.GET{})

			rawClient := client.NewClient(
				"http://local/graphql",
				clientopt.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
					return handlerRoundTripper{handler: srv}
				}),
			)

			// Query
			{
				size := 100
				userID := "1"
				userStatus := domain.StatusActive
				userOperation, err := rawClient.Post(ctx, query.UserOperationOp, query.UserOperationVars{
					ArticleID:    "article-1",
					MetadataID:   "metadata-1",
					Size:         graphql.OmittableOf[*int](&size),
					UserID:       graphql.OmittableOf[*string](&userID),
					UserStatus:   graphql.OmittableOf[*domain.Status](&userStatus),
					IncludeEmail: false,
					SkipName:     true,
				})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if diff := cmp.Diff(tt.want.userOperation, userOperation); diff != "" {
					t.Errorf("integrationTest mismatch (-want +got):\n%s", diff)
				}
			}
			// Query with list-type variables (旧 listvars: スカラーリスト [ID!]! / Input リスト
			// [FilterInput!]! / 入れ子リスト [[String!]] が構造を保ってエンコードされ、実サーバが
			// 受理することを検証する)
			{
				matrixRow := []string{"x", "y"}
				filteredKeys, err := rawClient.Post(ctx, query.FilteredKeysOp, query.FilteredKeysVars{
					Filters: []domain.FilterInput{{Key: "k", Value: "v"}},
					Ids:     []string{"1", "2"},
					Matrix:  graphql.OmittableOf(&[]*[]string{&matrixRow}),
				})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				// resolver は ids をそのまま返す
				if diff := cmp.Diff(&domain.FilteredKeys{FilteredKeys: []string{"1", "2"}}, filteredKeys); diff != "" {
					t.Errorf("filteredKeys mismatch (-want +got):\n%s", diff)
				}
			}
			// Query via GET
			{
				size := 100
				userID := "1"
				userStatus := domain.StatusActive
				userOperation, err := rawClient.Get(ctx, query.UserOperationOp, query.UserOperationVars{
					ArticleID:    "article-1",
					MetadataID:   "metadata-1",
					Size:         graphql.OmittableOf[*int](&size),
					UserID:       graphql.OmittableOf[*string](&userID),
					UserStatus:   graphql.OmittableOf[*domain.Status](&userStatus),
					IncludeEmail: false,
					SkipName:     true,
				})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if diff := cmp.Diff(tt.want.userOperation, userOperation); diff != "" {
					t.Errorf("integrationTest mismatch (-want +got):\n%s", diff)
				}
			}
			// Test field argument default values (schema-level defaults)
			{
				size := 100
				userOperation, err := rawClient.Post(ctx, query.UserOperationOp, query.UserOperationVars{
					ArticleID:    "article-1",
					MetadataID:   "metadata-1",
					Size:         graphql.OmittableOf[*int](&size),
					IncludeEmail: false,
					SkipName:     true,
				})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				// UserID / UserStatus を省略すると Omittable のゼロ値 = undefined になり、
				// omitzero で variables から除外される。GraphQL の schema default は変数が省略された
				// ときだけ適用されるため、resolver が default の "John Doe" を返すことを確認する。
				if userOperation.User.UserFragment2.Name != "John Doe" {
					t.Errorf("expected user name to be 'John Doe', got '%s'", userOperation.User.UserFragment2.Name)
				}
			}

			// @skip / @include を実サーバで評価する。include=true で emailIfIncluded、skip=false で
			// nameIfNotSkipped がレスポンスに含まれ、ポインタに値が入る(欠落時の nil と区別できる)。
			{
				userOperation, err := rawClient.Post(ctx, query.UserOperationOp, query.UserOperationVars{
					ArticleID:    "article-1",
					MetadataID:   "metadata-1",
					IncludeEmail: true,
					SkipName:     false,
				})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if userOperation.User.EmailIfIncluded == nil {
					t.Error("emailIfIncluded should be non-nil when @include(if: true)")
				}
				if userOperation.User.NameIfNotSkipped == nil {
					t.Error("nameIfNotSkipped should be non-nil when @skip(if: false)")
				}
			}

			// Mutation
			{
				input := domain.UpdateUserInput{
					ID:   "1",
					Name: graphql.OmittableOf[*string](nil),
				}
				updateUser, err := rawClient.Post(ctx, query.UpdateUserOp, query.UpdateUserVars{Input: input})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if updateUser.GetUpdateUser().User.Name != "nil" {
					t.Errorf("expected name to be 'nil', got '%s'", updateUser.GetUpdateUser().User.Name)
				}
			}
			{
				input := domain.UpdateUserInput{
					ID:   "1",
					Name: graphql.Omittable[*string]{},
				}
				updateUser, err := rawClient.Post(ctx, query.UpdateUserOp, query.UpdateUserVars{Input: input})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if updateUser.GetUpdateUser().User.Name != "undefined" {
					t.Errorf("expected name to be 'undefined', got '%s'", updateUser.GetUpdateUser().User.Name)
				}
			}
			{
				input := domain.UpdateUserInput{
					ID:   "1",
					Name: graphql.OmittableOf[*string](new("Sam Smith")),
				}
				updateUser, err := rawClient.Post(ctx, query.UpdateUserOp, query.UpdateUserVars{Input: input})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if updateUser.GetUpdateUser().User.Name != "Sam Smith" {
					t.Errorf("expected name to be 'Sam Smith', got '%s'", updateUser.GetUpdateUser().User.Name)
				}
			}
			// Test nested input object type (UserSettingsInput)
			{
				input := domain.UpdateUserInput{
					ID:   "1",
					Name: graphql.OmittableOf[*string](new("Test User")),
					Settings: graphql.OmittableOf[*domain.UserSettingsInput](&domain.UserSettingsInput{
						Theme:         "dark",
						Notifications: true,
					}),
				}
				updateUser, err := rawClient.Post(ctx, query.UpdateUserOp, query.UpdateUserVars{Input: input})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if updateUser.GetUpdateUser().User.Settings == nil {
					t.Errorf("expected settings to be set, got nil")
				}
				if updateUser.GetUpdateUser().User.Settings.Theme != "dark" {
					t.Errorf("expected theme to be 'dark', got '%s'", updateUser.GetUpdateUser().User.Settings.Theme)
				}
				if updateUser.GetUpdateUser().User.Settings.Notifications != true {
					t.Errorf("expected notifications to be true, got %v", updateUser.GetUpdateUser().User.Settings.Notifications)
				}
			}
			{
				input := domain.UpdateUserInput{
					ID:       "1",
					Name:     graphql.OmittableOf[*string](new("Test User")),
					Settings: graphql.OmittableOf[*domain.UserSettingsInput](nil),
				}
				updateUser, err := rawClient.Post(ctx, query.UpdateUserOp, query.UpdateUserVars{Input: input})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if updateUser.GetUpdateUser().User.Settings != nil {
					t.Errorf("expected settings to be nil, got %+v", updateUser.GetUpdateUser().User.Settings)
				}
			}
			{
				input := domain.UpdateUserInput{
					ID:       "1",
					Name:     graphql.OmittableOf[*string](new("Test User")),
					Settings: graphql.Omittable[*domain.UserSettingsInput]{},
				}
				updateUser, err := rawClient.Post(ctx, query.UpdateUserOp, query.UpdateUserVars{Input: input})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if updateUser.GetUpdateUser().User.Settings != nil {
					t.Errorf("expected settings to be nil (omitted), got %+v", updateUser.GetUpdateUser().User.Settings)
				}
			}
		})
	}
}

// query と client を同じディレクトリ(= 同一 Go パッケージ)へ出力すると、client_gen.go は
// レスポンス型と Document 定数を自パッケージ名で修飾せずに参照する。修飾すると自パッケージへの
// self-import になりコンパイルできない(v0 互換の1ディレクトリ構成が壊れる)ための回帰テスト。
func Test_IntegrationTest_SamePackage(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic: %v", r)
		}
	}()

	t.Chdir("testdata/integration/samepackage/")
	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// 修飾なしで生成されることをファイル内容で検証する
	compareFiles(t, "./want/client_gen.go.txt", "gen/client_gen.go")

	// コミット済みの gen パッケージを import して参照することで、生成コードが
	// コンパイル可能であることも担保する
	if samepackagegen.GetUserOp.Name != "GetUser" {
		t.Errorf("GetUserOp.Name = %q, want %q", samepackagegen.GetUserOp.Name, "GetUser")
	}
}

func compareFiles(t *testing.T, wantFile, generatedFile string) {
	t.Helper()

	// Compare file contents
	want, err := os.ReadFile(wantFile)
	if err != nil {
		t.Errorf("error reading file (expected file): %v", err)
		return
	}

	generated, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Errorf("error reading file (actual file): %v", err)
		return
	}

	if diff := cmp.Diff(string(want), string(generated)); diff != "" {
		t.Errorf("file contents differ:\n%s", diff)
	}
}

type handlerRoundTripper struct {
	handler http.Handler
}

func (rt handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	recorder := httptest.NewRecorder()
	rt.handler.ServeHTTP(recorder, reqClone)
	resp := recorder.Result()
	return resp, nil
}

func Test_Subscription(t *testing.T) {
	t.Parallel()

	type args struct {
		target int
	}

	type want struct {
		counts []int
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// サーバーが 1..target を順に push し、complete でストリームが終わる
			name: "countサブスクリプションが順に値を受け取れる",
			args: args{
				target: 3,
			},
			want: want{
				counts: []int{1, 2, 3},
			},
		},
		{
			// target が 1 の場合は 1 件だけ受け取って終わる
			name: "1件だけのサブスクリプション",
			args: args{
				target: 1,
			},
			want: want{
				counts: []int{1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// httptest.NewTestServer のインメモリ network と synctest の擬似クロックで
			// 実ポート・実時間に依存しない決定論的な subscription テストにする
			synctest.Test(t, func(t *testing.T) {
				es := schema.NewExecutableSchema(schema.Config{Resolvers: &schema.Resolver{}})
				srv := handler.New(es)
				srv.AddTransport(transport.Websocket{
					KeepAlivePingInterval: 10 * time.Second,
					InitTimeout:           5 * time.Second,
				})

				httpServer := httptest.NewTestServer(t, srv)
				// Client() は in-memory サーバを起動して httpServer.URL を設定するため、
				// URL を読む前に呼ぶ
				httpClient := httpServer.Client()

				subClient := client.NewSubscriptionClient(httpServer.URL, clientopt.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
					return httpClient.Transport
				}))

				ctx := t.Context()

				var got []int
				for res, err := range subClient.Subscribe(ctx, query.CountOp, query.CountVars{Target: tt.args.target}) {
					if err != nil {
						t.Fatalf("subscription error: %v", err)
					}
					got = append(got, res.Count)
				}

				if diff := cmp.Diff(tt.want.counts, got); diff != "" {
					t.Errorf("diff(-want +got): %s", diff)
				}
			})
		})
	}
}
