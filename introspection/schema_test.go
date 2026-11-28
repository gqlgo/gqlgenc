package introspection

import (
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/vektah/gqlparser/v2"
)

// mockRemoteServer は、capability の問い合わせと introspection クエリに別々の応答を返せるリモートサーバー。
// 受け取ったクエリとヘッダーを記録する。
type mockRemoteServer struct {
	URL string

	mutex   sync.Mutex
	header  http.Header
	queries []string
}

// newMockRemoteServer は mockRemoteServer を起動する。サーバーはテストの終わりに閉じる。
//
// introspection クエリには responseFile の内容を返す。capability の問い合わせには
// capabilityResponseFile の内容を返し、capabilityResponseFile が空なら responseFile の内容を返す。
// httpErrorStatus が 0 でなければ、どのクエリにもそのステータスのエラーを返す。
func newMockRemoteServer(t *testing.T, responseFile, capabilityResponseFile string, httpErrorStatus int) *mockRemoteServer {
	t.Helper()

	mock := &mockRemoteServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var request struct {
			OperationName string `json:"operationName"`
			Query         string `json:"query"`
		}
		if err := json.UnmarshalRead(req.Body, &request); err != nil {
			t.Errorf("failed to decode request: %v", err)

			return
		}

		mock.mutex.Lock()
		mock.header = req.Header.Clone()
		mock.queries = append(mock.queries, request.Query)
		mock.mutex.Unlock()

		if httpErrorStatus != 0 {
			w.WriteHeader(httpErrorStatus)
			if _, err := w.Write([]byte("Internal Server Error")); err != nil {
				t.Errorf("failed to write error response: %v", err)
			}

			return
		}

		file := responseFile
		if request.OperationName == "Capabilities" && capabilityResponseFile != "" {
			file = capabilityResponseFile
		}

		body, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("failed to read response file %s: %v", file, err)
		}
		if _, err := w.Write(body); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	mock.URL = server.URL

	return mock
}

// received は、サーバーが受け取ったクエリ (受け取った順) と、最後に受け取ったリクエストのヘッダーを返す。
func (m *mockRemoteServer) received() ([]string, http.Header) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return slices.Clone(m.queries), m.header.Clone()
}

func TestLoadRemoteSchema(t *testing.T) {
	t.Parallel()

	// withoutOptionalParts は、introspection の任意の部分を使わない読み込みでサーバーが受け取るクエリを返す
	withoutOptionalParts := func() []string {
		return []string{CapabilityQueryDocument, Introspection}
	}

	type args struct {
		// responseFile は introspection クエリへの応答
		responseFile string
		// capabilityResponseFile は capability の問い合わせへの応答。空なら responseFile を返す
		capabilityResponseFile string
		httpErrorStatus        int
		authorization          string
	}

	type want struct {
		err           error
		authorization string
		// queries は、サーバーが受け取ったクエリ (受け取った順)
		queries []string
		// validQuery は、取得したスキーマに対して妥当なクエリ。空なら検証しない
		validQuery string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 正常な introspection 結果からスキーマを構築できる。capability の問い合わせに
			// introspection の型の情報を返さないサーバーには、任意の部分を使わないクエリを送る
			name: "introspectionで成功する",
			args: args{responseFile: "testdata/remote/response_ok.json"},
			want: want{err: nil, queries: withoutOptionalParts()},
		},
		{
			// interface 実装を含む introspection 結果も構築できる
			name: "interface実装を含むスキーマで成功する",
			args: args{responseFile: "testdata/remote/response_with_implements.json"},
			want: want{err: nil, queries: withoutOptionalParts()},
		},
		{
			// フィールドの無い Query 型はバリデーションで弾く
			name: "不正なスキーマでバリデーションエラー",
			args: args{responseFile: "testdata/remote/response_invalid_schema.json"},
			want: want{err: errors.New("OBJECT Query: must define one or more fields"), queries: withoutOptionalParts()},
		},
		{
			// introspection リクエストが HTTP エラーを返した場合。capability の問い合わせの失敗では止まらず、
			// 続く introspection クエリの失敗を報告する
			name: "HTTPエラーでクエリ失敗",
			args: args{httpErrorStatus: http.StatusInternalServerError},
			want: want{err: errors.New("introspection query failed"), queries: withoutOptionalParts()},
		},
		{
			// endpoint.headers がリクエストに付与される
			name: "headersがリクエストに付与される",
			args: args{responseFile: "testdata/remote/response_ok.json", authorization: "Bearer test-token"},
			want: want{authorization: "Bearer test-token", queries: withoutOptionalParts()},
		},
		{
			// 現行のサーバー。deprecated な引数 (test の old) と input フィールド (Filter.old) を使い、
			// repeatable なディレクティブ (@tag) を同じ位置に2回書くクエリが、取得したスキーマで検証を通る
			name: "すべてのcapabilityを持つサーバーからdeprecatedな引数とinputフィールドとrepeatableなディレクティブを取得する",
			args: args{
				responseFile:           "testdata/remote/response_deprecated_inputs.json",
				capabilityResponseFile: "testdata/remote/response_capabilities_all.json",
			},
			want: want{
				queries: []string{CapabilityQueryDocument, NewQuery(Capabilities{
					FieldArgumentDeprecation:     true,
					InputFieldDeprecation:        true,
					DirectiveArgumentDeprecation: true,
					InputValueDeprecation:        true,
					RepeatableDirective:          true,
				})},
				validQuery: `query Test { test(old: true) @tag(name: "a") @tag(name: "b") search(filter: {old: true}) }`,
			},
		},
		{
			// 任意の部分ができる前のサーバー。使えない部分を含むクエリは拒否されるため、送らない
			name: "capabilityを持たないサーバーには任意の部分を含まないクエリを送る",
			args: args{
				responseFile:           "testdata/remote/response_ok.json",
				capabilityResponseFile: "testdata/remote/response_capabilities_none.json",
			},
			want: want{queries: withoutOptionalParts()},
		},
		{
			// capability の問い合わせにエラーを返すサーバー。問い合わせの失敗はエラーにしない
			name: "capabilityの問い合わせを拒否するサーバーには任意の部分を含まないクエリを送る",
			args: args{
				responseFile:           "testdata/remote/response_ok.json",
				capabilityResponseFile: "testdata/remote/response_capabilities_rejected.json",
			},
			want: want{queries: withoutOptionalParts()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newMockRemoteServer(t, tt.args.responseFile, tt.args.capabilityResponseFile, tt.args.httpErrorStatus)

			var header http.Header
			if tt.args.authorization != "" {
				header = http.Header{"Authorization": []string{tt.args.authorization}}
			}

			schema, err := LoadRemoteSchema(t.Context(), mock.URL, header)

			gotQueries, gotHeader := mock.received()
			if diff := cmp.Diff(tt.want.queries, gotQueries); diff != "" {
				t.Errorf("queries diff(-want +got): %s", diff)
			}

			if tt.want.err != nil {
				if err == nil {
					t.Fatalf("error = nil, want error containing %q", tt.want.err.Error())
				}
				if !strings.Contains(err.Error(), tt.want.err.Error()) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.want.err.Error())
				}

				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if schema == nil {
				t.Fatal("schema = nil, want non-nil")
			}
			if tt.want.authorization != "" && gotHeader.Get("Authorization") != tt.want.authorization {
				t.Errorf("Authorization = %q, want %q", gotHeader.Get("Authorization"), tt.want.authorization)
			}
			if tt.want.validQuery != "" {
				_, errs := gqlparser.LoadQueryWithRules(schema, tt.want.validQuery, nil)
				if diff := cmp.Diff("", errs.Error()); diff != "" {
					t.Errorf("query validation errors diff(-want +got): %s", diff)
				}
			}
		})
	}
}
