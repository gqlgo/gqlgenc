package client

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestDo(t *testing.T) {
	t.Parallel()

	type user struct {
		Name string `json:"name"`
	}

	type getUserVars struct {
		ID   string `json:"id"`
		Size *int   `json:"size"`
	}

	type args struct {
		op       Operation[getUserVars, user]
		vars     getUserVars
		respBody string
	}

	type want struct {
		requestBody string
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
				op: Operation[getUserVars, user]{
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
			// GraphQL エラーのレスポンスはエラーとして返る
			name: "GraphQLエラーのときはエラーを返す",
			args: args{
				op: Operation[getUserVars, user]{
					Name:     "GetUser",
					Document: "query GetUser($id: ID!) { user(id: $id) { name } }",
				},
				vars:     getUserVars{ID: "1"},
				respBody: `{"data":null,"errors":[{"message":"not found"}]}`,
			},
			want: want{
				requestBody: `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"1","size":null},"operationName":"GetUser"}`,
				err:         cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var gotRequestBody []byte
			httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				var err error
				gotRequestBody, err = io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(tt.args.respBody))),
					Header:     http.Header{},
				}, nil
			})}

			c := NewClient("https://example.com/graphql", WithHTTPClient(httpClient))

			got, err := Do(t.Context(), c, tt.args.op, tt.args.vars)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.requestBody, string(gotRequestBody)); diff != "" {
				t.Errorf("request body diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.user, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
