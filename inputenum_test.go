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
)

// Test_IntegrationTest_ModelGen は gqlgen.model を指定した構成 (use case 2) を検証する。
// gqlgenc が model_gen.go を生成し、その出力 (Input 型・Enum 型) が実際にコンパイル・動作する
// ことを確認する。実 gqlgen サーバとの往復 (スキーマ整合) は Test_IntegrationTest_NoModelGen が
// 担保するため、ここでは固定レスポンスのモック RoundTripper で代替する。
func Test_IntegrationTest_ModelGen(t *testing.T) {
	t.Chdir("testdata/integration/inputenum/")

	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	// 生成された model_gen.go (Input 型・Enum 型のみ。Object は含まない。custom scalar は
	// 対応型ありの Timestamp -> time.Time、対応型なしの Raw -> graphql.String にバインド)、
	// query_gen.go、client_gen.go が want スナップショットと一致することを確認する
	compareFiles(t, "./want/model_gen.go.txt", "domain/model_gen.go")
	compareFiles(t, "./want/query_gen.go.txt", "domain/query_gen.go")
	compareFiles(t, "./want/client_gen.go.txt", "query/client_gen.go")

	// 生成された Input / Enum / 応答型が動作することを確認する (入力エンコード + レスポンスデコード)
	captured := &bytes.Buffer{}
	rawClient := client.NewClient("http://local/graphql", clientopt.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
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
