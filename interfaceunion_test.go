package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/client"
	ifudomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/interfaceunion/domain"
	ifuquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/interfaceunion/query"
)

// Test_IntegrationTest_ModelGen_InterfaceUnion は gqlgen.model を指定した構成 (use case 2) で
// スキーマが interface / union を含む場合を検証する。生成される model_gen.go は Input 型と
// Enum 型のみで、interface (Node) / union (SearchResult) / object (User/Post) は除去される
// (mutateHook の build.Interfaces=nil 経路)。inline fragment の応答型は querygen が生成し、
// __typename で判別デコードできることを確認する。
func Test_IntegrationTest_ModelGen_InterfaceUnion(t *testing.T) {
	t.Chdir("testdata/integration/interfaceunion/")

	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// model_gen.go は SearchFilter (Input) と NodeKind (Enum) のみ。interface / union /
	// object が除去されることを want スナップショットで固定する。
	compareFiles(t, "./want/model_gen.go.txt", "domain/model_gen.go")
	compareFiles(t, "./want/query_gen.go.txt", "domain/query_gen.go")
	compareFiles(t, "./want/client_gen.go.txt", "query/client_gen.go")

	// union / interface への inline fragment 応答が __typename で正しく判別デコードされることを確認する
	captured := &bytes.Buffer{}
	rawClient := client.NewClient("http://local/graphql", client.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
		return &cannedRoundTripper{
			response: `{"data":{"search":[{"__typename":"User","id":"u1","name":"Alice","kind":"USER"},{"__typename":"Post","id":"p1","title":"Hello"}],"node":{"__typename":"User","id":"u1","name":"Alice"}}}`,
			captured: captured,
		}
	}))

	got, err := rawClient.Post(t.Context(), ifuquery.SearchOp, ifuquery.SearchVars{
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
				Typename: new("User"),
			},
			{
				Post: &struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				}{ID: "p1", Title: "Hello"},
				Typename: new("Post"),
			},
		},
		Node: &ifudomain.Search_Node{
			User: &struct {
				Name string `json:"name"`
			}{Name: "Alice"},
			Typename: new("User"),
			ID:       "u1",
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("response diff(-want +got): %s", diff)
	}

	// Input 型・Enum 型が variables として正しくエンコードされていることを確認する
	for _, want := range [][]byte{[]byte(`"keyword":"go"`), []byte(`"kind":"USER"`)} {
		if !bytes.Contains(captured.Bytes(), want) {
			t.Errorf("request body should contain %q, got: %s", want, captured.String())
		}
	}
}
