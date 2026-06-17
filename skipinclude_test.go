package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/client"
	"github.com/Yamashou/gqlgenc/v3/internal/clientopt"
	sidomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/skipinclude/domain"
	siquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/skipinclude/query"
)

// Test_IntegrationTest_SkipInclude は、@skip / @include 付きフィールドが非null型でも
// ポインタ(nullable)として生成され、レスポンスから欠落したとき nil にデコードされることを担保する。
func Test_IntegrationTest_SkipInclude(t *testing.T) {
	t.Chdir("testdata/integration/skipinclude/")

	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	compareFiles(t, "./want/query_gen.go.txt", "domain/query_gen.go")
	compareFiles(t, "./want/client_gen.go.txt", "query/client_gen.go")

	// @include した name は含み、@skip した description は欠落させた応答を返す。
	rawClient := client.NewClient("http://local/graphql", clientopt.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
		return &cannedRoundTripper{
			response: `{"data":{"item":{"id":"1","name":"foo","tags":["a","b"]}}}`,
			captured: &bytes.Buffer{},
		}
	}))

	got, err := rawClient.Post(t.Context(), siquery.GetItemOp, siquery.GetItemVars{
		WithName: true,
		SkipDesc: true,
	})
	if err != nil {
		t.Fatalf("Post error = %v", err)
	}

	// @include された name は値が入り、@skip で欠落した description は nil になる。
	want := &sidomain.GetItem{
		Item: sidomain.GetItem_Item{
			ID:          "1",
			Name:        new("foo"),
			Description: nil,
			Tags:        []string{"a", "b"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("response diff(-want +got): %s", diff)
	}
}
