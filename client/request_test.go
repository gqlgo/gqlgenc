package client

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNewPostRequest(t *testing.T) {
	tests := []struct {
		name          string
		endpoint      string
		operationName string
		query         string
		variables     map[string]any
		wantErr       bool
	}{
		{
			name:          "Basic request",
			endpoint:      "http://example.com/graphql",
			operationName: "TestQuery",
			query:         "query TestQuery { test }",
			variables:     nil,
			wantErr:       false,
		},
		{
			name:          "Request with variables",
			endpoint:      "http://example.com/graphql",
			operationName: "TestQuery",
			query:         "query TestQuery($id: ID!) { test(id: $id) }",
			variables:     map[string]any{"id": "123"},
			wantErr:       false,
		},
		{
			name:          "Complex request with variables",
			endpoint:      "http://example.com/graphql",
			operationName: "TestMutation",
			query:         "mutation TestMutation($input: UserInput!) { createUser(input: $input) { id name } }",
			variables: map[string]any{
				"input": map[string]any{
					"name":  "Test User",
					"email": "test@example.com",
				},
			},
			wantErr: false,
		},
		{
			name:          "Invalid endpoint",
			endpoint:      "://invalid-url",
			operationName: "TestQuery",
			query:         "query TestQuery { test }",
			variables:     nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			req, err := NewPostRequest(ctx, tt.endpoint, tt.operationName, tt.query, tt.variables)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if req == nil {
				t.Fatal("Expected non-nil request")
			}

			// リクエストの検証
			if req.Method != http.MethodPost {
				t.Errorf("Expected method %s, but got %s", http.MethodPost, req.Method)
			}
			if req.URL.String() != tt.endpoint {
				t.Errorf("Expected URL %s, but got %s", tt.endpoint, req.URL.String())
			}

			// ヘッダーの検証
			if contentType := req.Header.Get("Content-Type"); contentType != "application/json;charset=utf-8" {
				t.Errorf("Expected Content-Type %s, but got %s", "application/json;charset=utf-8", contentType)
			}
			acceptValues := req.Header.Values("Accept")
			if !slices.Contains(acceptValues, "application/graphql-response+json;charset=utf-8") {
				t.Errorf("Accept header should contain application/graphql-response+json;charset=utf-8")
			}
			if !slices.Contains(acceptValues, "application/json;charset=utf-8") {
				t.Errorf("Accept header should contain application/json;charset=utf-8")
			}

			// リクエストボディの検証
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("Failed to read request body: %v", err)
			}

			var requestBody Request
			err = json.Unmarshal(body, &requestBody)
			if err != nil {
				t.Fatalf("Failed to unmarshal request body: %v", err)
			}

			if requestBody.Query != tt.query {
				t.Errorf("Expected query %s, but got %s", tt.query, requestBody.Query)
			}
			if requestBody.OperationName != tt.operationName {
				t.Errorf("Expected operationName %s, but got %s", tt.operationName, requestBody.OperationName)
			}

			if tt.variables == nil {
				if requestBody.Variables != nil {
					t.Errorf("Expected nil variables, but got %v", requestBody.Variables)
				}
			} else {
				// 変数の検証
				actualVariables, ok := requestBody.Variables.(map[string]any)
				if !ok {
					t.Fatalf("Expected variables to be a map, but got %T", requestBody.Variables)
				}
				for key, expectedValue := range tt.variables {
					actualValue, ok := actualVariables[key]
					if !ok {
						t.Errorf("Variable %s not found", key)
						continue
					}

					// マップの場合は個別に検証
					expectedMap, expectedIsMap := expectedValue.(map[string]any)
					actualMap, actualIsMap := actualValue.(map[string]any)

					if expectedIsMap && actualIsMap {
						for k, v := range expectedMap {
							if !cmp.Equal(v, actualMap[k]) {
								t.Errorf("Variable %s.%s: expected %v, but got %v", key, k, v, actualMap[k])
							}
						}
					} else if !cmp.Equal(expectedValue, actualValue) {
						t.Errorf("Variable %s: expected %v, but got %v", key, expectedValue, actualValue)
					}
				}
			}
		})
	}
}

func TestNewGetRequest(t *testing.T) {
	t.Parallel()

	type args struct {
		operationName string
		query         string
		variables     any
	}

	type want struct {
		query         string
		operationName string
		variables     string
		hasVariables  bool
		err           error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// query と variables(JSON文字列)が URL クエリに載る
			name: "queryとvariablesがURLに載る",
			args: args{
				operationName: "GetUser",
				query:         "query GetUser($id: ID!) { user(id: $id) { name } }",
				variables:     map[string]any{"id": "1"},
			},
			want: want{
				query:         "query GetUser($id: ID!) { user(id: $id) { name } }",
				operationName: "GetUser",
				variables:     `{"id":"1"}`,
				hasVariables:  true,
			},
		},
		{
			// variables が nil の場合は variables パラメータを付けない
			name: "variablesがnilのときはvariablesパラメータを付けない",
			args: args{
				operationName: "Ping",
				query:         "query Ping { ping }",
				variables:     nil,
			},
			want: want{
				query:         "query Ping { ping }",
				operationName: "Ping",
				hasVariables:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := NewGetRequest(t.Context(), "http://example.com/graphql", tt.args.operationName, tt.args.query, tt.args.variables)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}
			if tt.want.err != nil {
				return
			}

			if req.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", req.Method)
			}
			if req.Body != nil {
				t.Errorf("GET request should have no body")
			}

			q := req.URL.Query()
			if got := q.Get("query"); got != tt.want.query {
				t.Errorf("query param = %q, want %q", got, tt.want.query)
			}
			if got := q.Get("operationName"); got != tt.want.operationName {
				t.Errorf("operationName param = %q, want %q", got, tt.want.operationName)
			}
			if _, ok := q["variables"]; ok != tt.want.hasVariables {
				t.Errorf("variables presence = %v, want %v", ok, tt.want.hasVariables)
			}
			if tt.want.hasVariables {
				if got := q.Get("variables"); got != tt.want.variables {
					t.Errorf("variables param = %q, want %q", got, tt.want.variables)
				}
			}
		})
	}
}
