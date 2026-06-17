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
// ポインタ(nullable)として生成され、レスポンスに含まれる/欠落するの両方が
// 値/nil として正しくデコードされることを担保する。
func Test_IntegrationTest_SkipInclude(t *testing.T) {
	t.Chdir("testdata/integration/skipinclude/")

	if err := run(t.Context()); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	compareFiles(t, "./want/query_gen.go.txt", "domain/query_gen.go")
	compareFiles(t, "./want/client_gen.go.txt", "query/client_gen.go")

	type args struct {
		vars     siquery.GetItemVars
		response string
	}
	type want struct {
		getItem *sidomain.GetItem
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// @include(if: true) で name は含まれ、@skip(if: true) で description は欠落する。
			name: "name は含まれ description は欠落する場合は Name=値 / Description=nil",
			args: args{
				vars:     siquery.GetItemVars{WithName: true, SkipDesc: true},
				response: `{"data":{"item":{"id":"1","name":"foo","tags":["a","b"]}}}`,
			},
			want: want{
				getItem: &sidomain.GetItem{
					Item: sidomain.GetItem_Item{
						ID:          "1",
						Name:        new("foo"),
						Description: nil,
						Tags:        []string{"a", "b"},
					},
				},
			},
		},
		{
			// @include(if: false) で name は欠落し、@skip(if: false) で description は含まれる。
			name: "name は欠落し description は含まれる場合は Name=nil / Description=値",
			args: args{
				vars:     siquery.GetItemVars{WithName: false, SkipDesc: false},
				response: `{"data":{"item":{"id":"2","description":"bar","tags":[]}}}`,
			},
			want: want{
				getItem: &sidomain.GetItem{
					Item: sidomain.GetItem_Item{
						ID:          "2",
						Name:        nil,
						Description: new("bar"),
						Tags:        []string{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawClient := client.NewClient("http://local/graphql", clientopt.WithRoundTripper(func(http.RoundTripper) http.RoundTripper {
				return &cannedRoundTripper{
					response: tt.args.response,
					captured: &bytes.Buffer{},
				}
			}))

			got, err := rawClient.Post(t.Context(), siquery.GetItemOp, tt.args.vars)
			if err != nil {
				t.Fatalf("Post error = %v", err)
			}

			if diff := cmp.Diff(tt.want.getItem, got); diff != "" {
				t.Errorf("response diff(-want +got): %s", diff)
			}
		})
	}
}
