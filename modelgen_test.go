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
	inputenumdomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/inputenum/domain"
	inputenumquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/inputenum/query"
	ifudomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/interfaceunion/domain"
	ifuquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/interfaceunion/query"
	lvdomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/listvars/domain"
	lvquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/listvars/query"
	nldomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/nestedlist/domain"
	nlquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/nestedlist/query"
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
			// Input 型・Enum 型の生成と custom scalar バインド (Timestamp -> time.Time、Raw -> graphql.String)
			name:     "input型 enum型 custom scalar の生成とエンコード/デコード",
			dir:      "testdata/integration/inputenum/",
			response: `{"data":{"search":{"total":42,"status":"ACTIVE"}}}`,
			check: func(t *testing.T, c *client.Client, captured *bytes.Buffer) {
				t.Helper()

				got, err := c.Post(t.Context(), inputenumquery.SearchOp, inputenumquery.SearchVars{
					Input: inputenumdomain.SearchInput{
						Keyword: "go",
						Status:  inputenumdomain.StatusActive,
						Page:    graphql.OmittableOf(&inputenumdomain.PageInput{Size: 10}),
					},
				})
				if err != nil {
					t.Fatalf("Post error = %v", err)
				}

				want := &inputenumdomain.Search{
					Search: inputenumdomain.Search_Search{
						Total:  42,
						Status: inputenumdomain.StatusActive,
					},
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("response diff(-want +got): %s", diff)
				}

				assertBodyContains(t, captured, `"keyword":"go"`, `"status":"ACTIVE"`, `"size":10`)
			},
		},
		{
			// interface (Node) / union (SearchResult) / object は model_gen.go から除去され、Input+Enum のみ生成。
			// inline fragment の応答型は querygen が生成し、__typename で判別デコードする。
			name:     "interface/union は model から除去され inline fragment は __typename で判別",
			dir:      "testdata/integration/interfaceunion/",
			response: `{"data":{"search":[{"__typename":"User","id":"u1","name":"Alice","kind":"USER"},{"__typename":"Post","id":"p1","title":"Hello"}],"node":{"__typename":"User","id":"u1","name":"Alice"}}}`,
			check: func(t *testing.T, c *client.Client, captured *bytes.Buffer) {
				t.Helper()

				got, err := c.Post(t.Context(), ifuquery.SearchOp, ifuquery.SearchVars{
					Filter: ifudomain.SearchFilter{
						Keyword: "go",
						Kind:    ifudomain.NodeKindUser,
					},
				})
				if err != nil {
					t.Fatalf("Post error = %v", err)
				}

				want := &ifudomain.Search{
					Search: []*ifudomain.Search_Search{
						{
							User: &struct {
								ID   string             `json:"id"`
								Kind ifudomain.NodeKind `json:"kind"`
								Name string             `json:"name"`
							}{ID: "u1", Kind: ifudomain.NodeKindUser, Name: "Alice"},
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
					Node: &ifudomain.Search_Node{
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

				assertBodyContains(t, captured, `"keyword":"go"`, `"kind":"USER"`)
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
		{
			// 入れ子リストの union ([[Cell!]!]!) を inline fragment で選択した応答が [][]*GetGrid_Grid
			// (要素は単一ポインタ) として生成され、__typename で判別デコードできる。ポインタをリスト段
			// ごとに重ねると [][]**GetGrid_Grid になる回帰を防ぐ。
			name:     "入れ子リストの union が単一ポインタで __typename 判別される",
			dir:      "testdata/integration/nestedlist/",
			response: `{"data":{"grid":[[{"__typename":"TextCell","text":"hi"},{"__typename":"NumberCell","number":5}]]}}`,
			check: func(t *testing.T, c *client.Client, _ *bytes.Buffer) {
				t.Helper()

				got, err := c.Post(t.Context(), nlquery.GetGridOp, nlquery.GetGridVars{
					Kind: nldomain.CellKindText,
				})
				if err != nil {
					t.Fatalf("Post error = %v", err)
				}

				want := &nldomain.GetGrid{
					Grid: [][]*nldomain.GetGrid_Grid{
						{
							{
								TextCell: &struct {
									Text string `json:"text"`
								}{Text: "hi"},
								Typename: "TextCell",
							},
							{
								NumberCell: &struct {
									Number int `json:"number"`
								}{Number: 5},
								Typename: "NumberCell",
							},
						},
					},
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("response diff(-want +got): %s", diff)
				}
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
