package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/99designs/gqlgen/graphql"

	"github.com/Yamashou/gqlgenc/v3/client"
	inputenumdomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/inputenum/domain"
	inputenumquery "github.com/Yamashou/gqlgenc/v3/testdata/integration/inputenum/query"
)

// Test_IntegrationTest_ModelGen は gqlgen.model を指定した構成 (use case 2) を検証する。
// gqlgenc が model_gen.go を生成し、その出力 (Input 型・Enum 型) が実際にコンパイル・動作する
// ことを確認する。サーバは固定レスポンスを返すモック RoundTripper で代替する。
func Test_IntegrationTest_ModelGen(t *testing.T) {
	t.Chdir("testdata/integration/inputenum/")

	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// 使われている Input 型と Enum 型だけを生成し、Object 型は生成しない
	declared := declaredTypes(t, "domain/model_gen.go")
	for _, name := range []string{"SearchInput", "PageInput", "Status"} {
		if !declared[name] {
			t.Errorf("model_gen.go should generate %q", name)
		}
	}
	for _, name := range []string{"SearchResult", "Query"} {
		if declared[name] {
			t.Errorf("model_gen.go should not generate object type %q", name)
		}
	}

	// 生成された Input / Enum / 応答型が動作することを確認する (入力エンコード + レスポンスデコード)
	captured := &bytes.Buffer{}
	rawClient := client.NewClient("http://local/graphql", client.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
		return &cannedRoundTripper{
			response: `{"data":{"search":{"total":42,"status":"ACTIVE"}}}`,
			captured: captured,
		}
	}))

	got, err := rawClient.Post(t.Context(), inputenumquery.SearchOp, inputenumquery.SearchVars{
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

	// Input 型・Enum 型が variables として正しくエンコードされていることを確認する
	for _, want := range [][]byte{[]byte(`"keyword":"go"`), []byte(`"status":"ACTIVE"`), []byte(`"size":10`)} {
		if !bytes.Contains(captured.Bytes(), want) {
			t.Errorf("request body should contain %q, got: %s", want, captured.String())
		}
	}
}

func declaredTypes(t *testing.T, goFile string) map[string]bool {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), goFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", goFile, err)
	}

	declared := make(map[string]bool)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				declared[typeSpec.Name.Name] = true
			}
		}
	}

	return declared
}

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
