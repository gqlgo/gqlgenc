package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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
					Article: &domain.UserOperation_Article{
						ID:    "article-1",
						Title: "Test Article",
						Tags:  []string{"tag1", "tag2", "tag3"},
						OptionalTags: &[]string{"optional1", "optional2"},
						Comments: []*domain.UserOperation_Article_Comments{
							{ID: "1", Text: "First comment"},
							{ID: "2", Text: "Second comment"},
						},
						OptionalComments: &[]*domain.UserOperation_Article_OptionalComments{
							{ID: "3", Text: "Optional comment"},
						},
						Rating:         4.5,
						OptionalRating: ptr(3.8),
						NullableElementsList: []*string{
							ptr("element1"),
							nil,
							ptr("element2"),
						},
						FullyNullableList: &[]*string{
							ptr("nullable1"),
							nil,
						},
						Statuses:         []domain.Status{domain.StatusActive, domain.StatusInactive},
						OptionalStatuses: &[]domain.Status{domain.StatusActive},
						Addresses: []*domain.UserOperation_Article_Addresses{
							{
								Street: "Public St",
								PublicAddress: struct {
									Public bool "json:\"public,omitempty,omitzero\""
								}{Public: true},
							},
							{
								Street: "Private St",
								PrivateAddress: struct {
									Private bool "json:\"private,omitempty,omitzero\""
								}{Private: true},
							},
						},
						OptionalAddresses: &[]*domain.UserOperation_Article_OptionalAddresses{
							{
								Street: "Optional St",
								PublicAddress: struct {
									Public bool "json:\"public,omitempty,omitzero\""
								}{Public: false},
							},
						},
						Profiles: []*domain.UserOperation_Article_Profiles{
							{
								PublicProfile: struct {
									Status domain.Status "json:\"status,omitempty,omitzero\""
								}{Status: domain.StatusActive},
							},
							{
								PrivateProfile: struct {
									Age *int "json:\"age\""
								}{Age: ptr(25)},
							},
						},
						OptionalProfiles: &[]*domain.UserOperation_Article_OptionalProfiles{
							{
								PublicProfile: struct {
									Status domain.Status "json:\"status,omitempty,omitzero\""
								}{Status: domain.StatusInactive},
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
						Data: ptr(`{"key":"value","number":123}`),
					},
					OptionalUser: &domain.UserOperation_OptionalUser{
						Name: "Sam Smith",
					},
					User: domain.UserOperation_User{
						User: struct {
							domain.UserFragment2 `json:"-"`
							Name                 string "json:\"name,omitempty,omitzero\""
						}{
							UserFragment2: domain.UserFragment2{Name: "John Doe"},
							Name:          "John Doe",
						},
						UserFragment1: domain.UserFragment1{
							User: struct {
								Name string "json:\"name,omitempty,omitzero\""
							}{
								Name: "John Doe",
							},
							Name: "John Doe",
							Profile: domain.UserFragment1_Profile{
								PrivateProfile: struct {
									Age *int "json:\"age\""
								}{
									Age: func() *int { i := 30; return &i }(),
								},
							},
						},
						UserFragment2: domain.UserFragment2{Name: "John Doe"},
						Name:          "John Doe",
						Name2:         "John Doe",
						Address: domain.UserOperation_User_Address{
							Street: "123 Main St",
							PrivateAddress: struct {
								Private bool   "json:\"private,omitempty,omitzero\""
								Street  string "json:\"street,omitempty,omitzero\""
							}{
								Street: "123 Main St",
							},
							PublicAddress: struct {
								Public bool   "json:\"public,omitempty,omitzero\""
								Street string "json:\"street,omitempty,omitzero\""
							}{
								Street: "123 Main St",
							},
						},
						Profile: domain.UserOperation_User_Profile{
							PrivateProfile: struct {
								Age *int "json:\"age\""
							}{
								Age: func() *int { i := 30; return &i }(),
							},
						},
						Profile2: domain.UserOperation_User_Profile2{
							PrivateProfile: struct {
								Age *int "json:\"age\""
							}{
								Age: func() *int { i := 30; return &i }(),
							},
						},
						OptionalProfile: &domain.UserOperation_User_OptionalProfile{
							PublicProfile: struct {
								Status domain.Status "json:\"status,omitempty,omitzero\""
							}{
								Status: domain.StatusActive,
							},
						},
						OptionalAddress: &domain.UserOperation_User_OptionalAddress{
							Street: "456 Elm St",
							PrivateAddress: struct {
								Private bool   "json:\"private,omitempty,omitzero\""
								Street  string "json:\"street,omitempty,omitzero\""
							}{
								Street: "456 Elm St",
							},
							PublicAddress: struct {
								Public bool   "json:\"public,omitempty,omitzero\""
								Street string "json:\"street,omitempty,omitzero\""
							}{
								Street: "456 Elm St",
							},
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
			err := run()
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

			httpClient := &http.Client{Transport: handlerRoundTripper{handler: srv}}
			c := query.NewClient(client.NewClient(
				"http://local/graphql",
				client.WithHTTPClient(httpClient),
			))

			// Query
			{
				userOperation, err := c.UserOperation(ctx, "article-1", "metadata-1")
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if diff := cmp.Diff(tt.want.userOperation, userOperation); diff != "" {
					t.Errorf("integrationTest mismatch (-want +got):\n%s", diff)
				}
			}

			// Mutation
			{
				input := domain.UpdateUserInput{
					ID:   "1",
					Name: graphql.OmittableOf[*string](nil),
				}
				updateUser, err := c.UpdateUser(ctx, input)
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
				updateUser, err := c.UpdateUser(ctx, input)
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
					Name: graphql.OmittableOf[*string](ptr("Sam Smith")),
				}
				updateUser, err := c.UpdateUser(ctx, input)
				if err != nil {
					t.Errorf("request failed: %v", err)
				}
				if updateUser.GetUpdateUser().User.Name != "Sam Smith" {
					t.Errorf("expected name to be 'Sam Smith', got '%s'", updateUser.GetUpdateUser().User.Name)
				}
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
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
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	reqClone := req.Clone(req.Context())
	reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	recorder := httptest.NewRecorder()
	rt.handler.ServeHTTP(recorder, reqClone)
	resp := recorder.Result()
	return resp, nil
}
