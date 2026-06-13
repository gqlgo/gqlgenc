package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestPost(t *testing.T) {
	t.Parallel()

	type user struct {
		Name string `json:"name"`
	}

	type getUserVars struct {
		ID   string `json:"id"`
		Size *int   `json:"size"`
	}

	type args struct {
		op       Operation[Query, getUserVars, user]
		vars     getUserVars
		respBody string
		options  []Option
	}

	type want struct {
		requestBody string
		header      http.Header
		user        *user
		err         error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 型付き変数が variables メンバーとしてエンコードされ、レスポンスの data がデコードされる
			name: "型付き変数でオペレーションを実行できる",
			args: args{
				op: Operation[Query, getUserVars, user]{
					Name:     "GetUser",
					Document: "query GetUser($id: ID!) { user(id: $id) { name } }",
				},
				vars:     getUserVars{ID: "1"},
				respBody: `{"data":{"name":"John Doe"}}`,
			},
			want: want{
				requestBody: `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"1","size":null},"operationName":"GetUser"}`,
				user:        &user{Name: "John Doe"},
			},
		},
		{
			// GraphQL エラーのレスポンスはエラーになるが、同時に返された部分データもデコードされる
			name: "GraphQLエラーのときはエラーと部分データを返す",
			args: args{
				op: Operation[Query, getUserVars, user]{
					Name:     "GetUser",
					Document: "query GetUser($id: ID!) { user(id: $id) { name } }",
				},
				vars:     getUserVars{ID: "1"},
				respBody: `{"data":{"name":"Partial"},"errors":[{"message":"not found"}]}`,
			},
			want: want{
				requestBody: `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"1","size":null},"operationName":"GetUser"}`,
				user:        &user{Name: "Partial"},
				err:         cmpopts.AnyError,
			},
		},
		{
			// 呼び出し単位の WithHTTPHeader はそのリクエストに付与される
			name: "WithHTTPHeaderで設定したヘッダーがリクエストに付与される",
			args: args{
				op: Operation[Query, getUserVars, user]{
					Name:     "GetUser",
					Document: "query GetUser($id: ID!) { user(id: $id) { name } }",
				},
				vars:     getUserVars{ID: "1"},
				respBody: `{"data":{"name":"John Doe"}}`,
				options: []Option{
					WithHTTPHeader(http.Header{
						"Authorization": []string{"Bearer token"},
						"x-request-id":  []string{"req-1"},
					}),
				},
			},
			want: want{
				requestBody: `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"1","size":null},"operationName":"GetUser"}`,
				header: http.Header{
					"Content-Type":  []string{"application/json;charset=utf-8"},
					"Accept":        []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
					"Authorization": []string{"Bearer token"},
					"X-Request-Id":  []string{"req-1"},
				},
				user: &user{Name: "John Doe"},
			},
		},
		{
			// 同名キーを指定するとデフォルトヘッダーをキー単位で上書きできる
			name: "WithHTTPHeaderでデフォルトのContent-Typeをキー単位で上書きできる",
			args: args{
				op: Operation[Query, getUserVars, user]{
					Name:     "GetUser",
					Document: "query GetUser($id: ID!) { user(id: $id) { name } }",
				},
				vars:     getUserVars{ID: "1"},
				respBody: `{"data":{"name":"John Doe"}}`,
				options: []Option{
					WithHTTPHeader(http.Header{
						"Content-Type": []string{"application/json"},
					}),
				},
			},
			want: want{
				requestBody: `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"1","size":null},"operationName":"GetUser"}`,
				header: http.Header{
					"Content-Type": []string{"application/json"},
					"Accept":       []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
				},
				user: &user{Name: "John Doe"},
			},
		},
		{
			// オプション未指定のときはデフォルトヘッダーのみが送信される
			name: "ヘッダー未設定のときはデフォルトヘッダーのみが送信される",
			args: args{
				op: Operation[Query, getUserVars, user]{
					Name:     "GetUser",
					Document: "query GetUser($id: ID!) { user(id: $id) { name } }",
				},
				vars:     getUserVars{ID: "1"},
				respBody: `{"data":{"name":"John Doe"}}`,
			},
			want: want{
				requestBody: `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"1","size":null},"operationName":"GetUser"}`,
				header: http.Header{
					"Content-Type": []string{"application/json;charset=utf-8"},
					"Accept":       []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
				},
				user: &user{Name: "John Doe"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var gotRequestBody []byte
			var gotHeader http.Header
			httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				gotHeader = req.Header
				var err error
				gotRequestBody, err = io.ReadAll(req.Body)
				if err != nil {
					return nil, fmt.Errorf("read request body: %w", err)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(tt.args.respBody))),
					Header:     http.Header{},
				}, nil
			})}

			c := NewClient("https://example.com/graphql", WithHTTPClient(httpClient))

			got, err := c.Post(t.Context(), tt.args.op, tt.args.vars, tt.args.options...)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.requestBody, string(gotRequestBody)); diff != "" {
				t.Errorf("request body diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.user, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}

			if tt.want.header != nil {
				if diff := cmp.Diff(tt.want.header, gotHeader); diff != "" {
					t.Errorf("header diff(-want +got): %s", diff)
				}
			}
		})
	}
}
