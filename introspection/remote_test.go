package introspection

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type mockRemoteServer struct {
	URL    string
	header http.Header
}

func newMockRemoteServer(t *testing.T, responseFile string) (*mockRemoteServer, func()) {
	t.Helper()

	mock := &mockRemoteServer{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mock.header = req.Header.Clone()

		body, err := os.ReadFile(responseFile)
		if err != nil {
			t.Errorf("failed to read response file %s: %v", responseFile, err)
		}
		if _, err := w.Write(body); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	mock.URL = server.URL

	return mock, server.Close
}

func newMockRemoteServerWithError(t *testing.T, statusCode int) (*mockRemoteServer, func()) {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statusCode)
		if _, err := w.Write([]byte("Internal Server Error")); err != nil {
			t.Errorf("failed to write error response: %v", err)
		}
	})

	server := httptest.NewServer(handler)

	return &mockRemoteServer{URL: server.URL}, server.Close
}

func TestLoadRemoteSchema(t *testing.T) {
	t.Parallel()

	type args struct {
		responseFile    string
		httpErrorStatus int
		authorization   string
	}

	type want struct {
		err           error
		authorization string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 正常な introspection 結果からスキーマを構築できる
			name: "introspectionで成功する",
			args: args{responseFile: "testdata/remote/response_ok.json"},
			want: want{err: nil},
		},
		{
			// interface 実装を含む introspection 結果も構築できる
			name: "interface実装を含むスキーマで成功する",
			args: args{responseFile: "testdata/remote/response_with_implements.json"},
			want: want{err: nil},
		},
		{
			// フィールドの無い Query 型はバリデーションで弾く
			name: "不正なスキーマでバリデーションエラー",
			args: args{responseFile: "testdata/remote/response_invalid_schema.json"},
			want: want{err: errors.New("OBJECT Query: must define one or more fields")},
		},
		{
			// introspection リクエストが HTTP エラーを返した場合
			name: "HTTPエラーでクエリ失敗",
			args: args{httpErrorStatus: http.StatusInternalServerError},
			want: want{err: errors.New("introspection query failed")},
		},
		{
			// schema.Query が null のとき Query 型を補完する
			name: "schema.QueryがnullでもQuery型を補完する",
			args: args{responseFile: "testdata/remote/response_query_null.json"},
			want: want{err: nil},
		},
		{
			// endpoint.headers がリクエストに付与される
			name: "headersがリクエストに付与される",
			args: args{responseFile: "testdata/remote/response_ok.json", authorization: "Bearer test-token"},
			want: want{authorization: "Bearer test-token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var mock *mockRemoteServer
			var closeServer func()
			if tt.args.httpErrorStatus != 0 {
				mock, closeServer = newMockRemoteServerWithError(t, tt.args.httpErrorStatus)
			} else {
				mock, closeServer = newMockRemoteServer(t, tt.args.responseFile)
			}
			defer closeServer()

			var header http.Header
			if tt.args.authorization != "" {
				header = http.Header{"Authorization": []string{tt.args.authorization}}
			}

			schema, err := LoadRemoteSchema(t.Context(), mock.URL, header)

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
				t.Error("schema = nil, want non-nil")
			}
			if tt.want.authorization != "" && mock.header.Get("Authorization") != tt.want.authorization {
				t.Errorf("Authorization = %q, want %q", mock.header.Get("Authorization"), tt.want.authorization)
			}
		})
	}
}
