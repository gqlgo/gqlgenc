package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/domain"
	"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/query"
	"github.com/Yamashou/gqlgenc/v3/testdata/integration/basic/schema"
)

func Test_IntegrationTest(t *testing.T) {
	type want struct {
		file          string
		userOperation *domain.UserOperation
	}
	tests := []struct {
		name    string
		testDir string
		wantErr bool
		want    want
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
								Street: "Optional St",
								PublicAddress: &struct {
									Public bool "json:\"public\""
								}{Public: false},
								Typename: new("PublicAddress"),
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
								PublicProfile: &struct {
									Status domain.Status "json:\"status\""
								}{Status: domain.StatusInactive},
								Typename: new("PublicProfile"),
							},
						},
						Matrix: [][]string{
							{"a", "b", "c"},
							{"d", "e", "f"},
						},
						OptionalMatrix: &[][]string{
							{"x", "y"},
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
						User: &struct {
							domain.UserFragment2 `json:"-"`

							Name string "json:\"name\""
						}{
							UserFragment2: domain.UserFragment2{Name: "John Doe"},
							Name:          "John Doe",
						},
						UserFragment1: domain.UserFragment1{
							User: &struct {
								Name string "json:\"name\""
							}{
								Name: "John Doe",
							},
							Typename: new("User"),
							Name:     "John Doe",
							Profile: domain.UserFragment1_Profile{
								PrivateProfile: &struct {
									Age *int "json:\"age\""
								}{
									Age: func() *int { i := 30; return &i }(),
								},
								Typename: new("PrivateProfile"),
							},
						},
						UserFragment2: domain.UserFragment2{Name: "John Doe"},
						Typename:      new("User"),
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
							PrivateProfile: &struct {
								Age *int "json:\"age\""
							}{
								Age: func() *int { i := 30; return &i }(),
							},
							Typename: new("PrivateProfile"),
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
							PublicAddress: &struct {
								Public bool   "json:\"public\""
								Street string "json:\"street\""
							}{
								Street: "456 Elm St",
							},
							Typename: new("PublicAddress"),
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
			// foo_bar と fooBar が同じ Go フィールド名 FooBar になる衝突 (gqlgo/gqlgenc#108)
			name:    "duplicate fields test - should fail due to Go field name collision",
			testDir: "testdata/integration/duplicate-fields/",
			wantErr: true,
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
				if err == nil {
					t.Errorf("run() expected error but got nil")
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
				client.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
					return handlerRoundTripper{handler: srv}
				}),
			)

			// Query
			{
				size := 100
				userID := "1"
				userStatus := domain.StatusActive
				userOperation, err := rawClient.Post(ctx, query.UserOperationOp, query.UserOperationVars{
					ArticleID:  "article-1",
					MetadataID: "metadata-1",
					Size:       &size,
					UserID:     &userID,
					UserStatus: &userStatus,
				})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if diff := cmp.Diff(tt.want.userOperation, userOperation); diff != "" {
					t.Errorf("integrationTest mismatch (-want +got):\n%s", diff)
				}
			}
			// Query via GET
			{
				size := 100
				userID := "1"
				userStatus := domain.StatusActive
				userOperation, err := rawClient.Get(ctx, query.UserOperationOp, query.UserOperationVars{
					ArticleID:  "article-1",
					MetadataID: "metadata-1",
					Size:       &size,
					UserID:     &userID,
					UserStatus: &userStatus,
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
					ArticleID:  "article-1",
					MetadataID: "metadata-1",
					Size:       &size,
				})
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				// When nil is passed, the resolver will use default value
				// Note: In GraphQL, schema-level defaults only apply when variables are omitted,
				// not when explicitly set to null. However, we verify the resolver behavior here.
				if userOperation.User.UserFragment2.Name != "John Doe" {
					t.Errorf("expected user name to be 'John Doe', got '%s'", userOperation.User.UserFragment2.Name)
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

				subClient := client.NewSubscriptionClient(httpServer.URL, client.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
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
