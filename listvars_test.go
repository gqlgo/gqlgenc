package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/client"
	lvdomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/listvars/domain"
	lvquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/listvars/query"
)

// Test_IntegrationTest_ModelGen_ListVars は、リスト型のクエリ変数 (スカラーリスト [ID!]!、
// 入れ子リスト [[String!]]、Input オブジェクトのリスト [FilterInput!]!) が Vars 構造体で
// リスト構造を保って生成され、リストとして正しくエンコードされることを検証する。
// operationArguments が要素の named 型だけを解決していた頃は構造を失い string になっていた。
func Test_IntegrationTest_ModelGen_ListVars(t *testing.T) {
	t.Chdir("testdata/integration/listvars/")

	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// Vars が Filters []FilterInput / Ids []string / Matrix *[]*[]string とリスト構造を保つ
	// ことを want スナップショットで固定する。
	compareFiles(t, "./want/model_gen.go.txt", "domain/model_gen.go")
	compareFiles(t, "./want/query_gen.go.txt", "domain/query_gen.go")
	compareFiles(t, "./want/client_gen.go.txt", "query/client_gen.go")

	captured := &bytes.Buffer{}
	rawClient := client.NewClient("http://local/graphql", client.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
		return &cannedRoundTripper{
			response: `{"data":{"search":["a","b"]}}`,
			captured: captured,
		}
	}))

	got, err := rawClient.Post(t.Context(), lvquery.SearchOp, lvquery.SearchVars{
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

	// リスト型変数がスカラーに潰れず、配列としてエンコードされることを確認する
	for _, want := range [][]byte{
		[]byte(`"ids":["1","2"]`),
		[]byte(`"filters":[{"key":"k","value":"v"}]`),
	} {
		if !bytes.Contains(captured.Bytes(), want) {
			t.Errorf("request body should contain %q, got: %s", want, captured.String())
		}
	}
}
