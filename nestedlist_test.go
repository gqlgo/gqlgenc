package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/internal/clientopt"
	nldomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/nestedlist/domain"
	nlquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/nestedlist/query"
)

// Test_IntegrationTest_ModelGen_NestedList は、入れ子リストの union ([[Cell!]!]!) を inline
// fragment で選択した応答が [][]*GetGrid_Grid (要素は単一ポインタ) として生成され、__typename で
// 判別デコードできることを検証する。object のリスト要素はポインタだが、ポインタはリスト段ごとに
// 重ねるのではなく一度だけ付くこと (リグレッションすると [][]**GetGrid_Grid になる) を担保する。
func Test_IntegrationTest_ModelGen_NestedList(t *testing.T) {
	t.Chdir("testdata/integration/nestedlist/")

	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// Grid が [][]*GetGrid_Grid (単一ポインタ) であることを want スナップショットで固定する。
	compareFiles(t, "./want/model_gen.go.txt", "domain/model_gen.go")
	compareFiles(t, "./want/query_gen.go.txt", "domain/query_gen.go")
	compareFiles(t, "./want/client_gen.go.txt", "query/client_gen.go")

	// 入れ子リストの union 応答が __typename で正しく判別デコードされることを確認する
	captured := &bytes.Buffer{}
	rawClient := client.NewClient("http://local/graphql", clientopt.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
		return &cannedRoundTripper{
			response: `{"data":{"grid":[[{"__typename":"TextCell","text":"hi"},{"__typename":"NumberCell","number":5}]]}}`,
			captured: captured,
		}
	}))

	got, err := rawClient.Post(t.Context(), nlquery.GetGridOp, nlquery.GetGridVars{
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
}
