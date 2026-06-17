package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/99designs/gqlgen/graphql"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/internal/clientopt"
	lvdomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/listvars/domain"
	lvquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/listvars/query"
	mgdomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/modelgen/domain"
	mgquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/modelgen/query"
)

// Test_IntegrationTest_ModelGen は generate.model.file を指定した構成 (use case 2) を検証する。
// gqlgenc が model_gen.go (Input 型・Enum 型) を生成し、生成物が want スナップショットと一致し、
// 実際にコンパイル・エンコード・デコードできることを確認する。実 gqlgen サーバとの往復 (スキーマ
// 整合) は Test_IntegrationTest_NoModelGen が担保するため、ここでは固定レスポンスのモック
// RoundTripper で代替する。
func Test_IntegrationTest_ModelGen(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		response string
		// check は生成済みクライアントで型付き Post を実行し、デコード結果と送信ボディを検証する。
		check func(t *testing.T, c *client.Client, captured *bytes.Buffer)
	}{
		{
			// generate.model.file 指定 (UC2)。model_gen.go は Input 型 (SearchInput/FilterInput/
			// PageInput/SearchFilter) と Enum 型 (Status/NodeKind) のみで、interface (Node) /
			// union (SearchResult) / object は除去される (mutateHook の build.Interfaces=nil 経路)。
			// custom scalar は Timestamp -> time.Time、Raw -> graphql.String にバインド。inline
			// fragment の応答型は querygen が生成し、__typename で判別デコードする。
			name:     "Input/Enum/custom scalar の生成と interface/union の model 除去",
			dir:      "testdata/integration/modelgen/",
			response: `{"data":{"searchInput":{"total":42,"status":"ACTIVE"},"searchNodes":[{"__typename":"User","id":"u1","name":"Alice","kind":"USER"},{"__typename":"Post","id":"p1","title":"Hello"}],"node":{"__typename":"User","id":"u1","name":"Alice"}}}`,
			check: func(t *testing.T, c *client.Client, captured *bytes.Buffer) {
				t.Helper()

				got, err := c.Post(t.Context(), mgquery.SearchOp, mgquery.SearchVars{
					Input: mgdomain.SearchInput{
						Keyword: "go",
						Status:  mgdomain.StatusActive,
						Page:    graphql.OmittableOf(&mgdomain.PageInput{Size: 10}),
					},
					Filter: mgdomain.SearchFilter{
						Keyword: "go",
						Kind:    mgdomain.NodeKindUser,
					},
				})
				if err != nil {
					t.Fatalf("Post error = %v", err)
				}

				want := &mgdomain.Search{
					SearchInput: mgdomain.Search_SearchInput{
						Status: mgdomain.StatusActive,
						Total:  42,
					},
					SearchNodes: []*mgdomain.Search_SearchNodes{
						{
							User: &struct {
								ID   string            `json:"id"`
								Kind mgdomain.NodeKind `json:"kind"`
								Name string            `json:"name"`
							}{ID: "u1", Kind: mgdomain.NodeKindUser, Name: "Alice"},
							Typename: "User",
						},
						{
							Post: &struct {
								ID    string `json:"id"`
								Title string `json:"title"`
							}{ID: "p1", Title: "Hello"},
							Typename: "Post",
						},
					},
					Node: &mgdomain.Search_Node{
						User: &struct {
							Name string `json:"name"`
						}{Name: "Alice"},
						Typename: "User",
						ID:       "u1",
					},
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("response diff(-want +got): %s", diff)
				}

				assertBodyContains(t, captured, `"keyword":"go"`, `"status":"ACTIVE"`, `"size":10`, `"kind":"USER"`)
			},
		},
		{
			// リスト型のクエリ変数 (スカラーリスト [ID!]!、Input オブジェクトのリスト [FilterInput!]!) が
			// Vars 構造体でリスト構造を保ち、配列としてエンコードされる。
			name:     "リスト型の変数が構造を保ってエンコードされる",
			dir:      "testdata/integration/listvars/",
			response: `{"data":{"search":["a","b"]}}`,
			check: func(t *testing.T, c *client.Client, captured *bytes.Buffer) {
				t.Helper()

				got, err := c.Post(t.Context(), lvquery.SearchOp, lvquery.SearchVars{
					Filters: []lvdomain.FilterInput{{Key: "k", Value: "v"}},
					Ids:     []string{"1", "2"},
				})
				if err != nil {
					t.Fatalf("Post error = %v", err)
				}

				want := &lvdomain.Search{Search: []string{"a", "b"}}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("response diff(-want +got): %s", diff)
				}

				assertBodyContains(t, captured, `"ids":["1","2"]`, `"filters":[{"key":"k","value":"v"}]`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(tt.dir)

			if err := run(t.Context()); err != nil {
				t.Fatalf("run() error = %v", err)
			}

			// 生成物 (model_gen.go は Input 型・Enum 型のみ) が want スナップショットと一致することを固定する。
			compareFiles(t, "./want/model_gen.go.txt", "domain/model_gen.go")
			compareFiles(t, "./want/query_gen.go.txt", "domain/query_gen.go")
			compareFiles(t, "./want/client_gen.go.txt", "query/client_gen.go")

			captured := &bytes.Buffer{}
			rawClient := client.NewClient("http://local/graphql", clientopt.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
				return &cannedRoundTripper{response: tt.response, captured: captured}
			}))

			tt.check(t, rawClient, captured)
		})
	}
}

// assertBodyContains は送信されたリクエストボディに各 want 部分文字列が含まれることを確認する。
func assertBodyContains(t *testing.T, captured *bytes.Buffer, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !bytes.Contains(captured.Bytes(), []byte(want)) {
			t.Errorf("request body should contain %q, got: %s", want, captured.String())
		}
	}
}

// cannedRoundTripper は固定レスポンスを返し、送信されたリクエストボディを captured に記録する。
type cannedRoundTripper struct {
	response string
	captured *bytes.Buffer
}

func (rt *cannedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		rt.captured.Write(body)
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(rt.response))),
	}, nil
}
