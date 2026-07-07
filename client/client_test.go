package client

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Yamashou/gqlgenc/v3/internal/cmputil"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestClient_unmarshalResponse(t *testing.T) {
	type testUser struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type fields struct {
		client   *http.Client
		endpoint string
	}

	type args struct {
		out      any
		respBody []byte
	}

	type want struct {
		data any
		err  error
	}

	tests := []struct {
		name   string
		fields fields
		want   want
		args   args
	}{
		{
			name: "Successful response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(`{"data":{"user":{"id":"1","name":"John Doe"}}}`),
				out: &struct {
					User testUser `json:"user"`
				}{},
			},
			want: want{
				data: &struct {
					User testUser `json:"user"`
				}{
					User: testUser{
						ID:   "1",
						Name: "John Doe",
					},
				},
				err: nil,
			},
		},
		{
			name: "Response with errors",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(`{"errors":[{"message":"Field not found","path":["user"]}],"data":null}`),
				out:      &map[string]any{},
			},
			want: want{
				data: &map[string]any{},
				err:  gqlerror.List{{Message: "Field not found", Path: ast.Path{ast.PathName("user")}}},
			},
		},
		{
			name: "Invalid response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(`{"data":invalid_json}`),
				out:      &map[string]any{},
			},
			want: want{
				data: &map[string]any{},
				err:  errors.New(`failed to decode response data: jsontext: invalid character 'i' at start of value within "/data" after offset 8`),
			},
		},
		{
			name: "Empty response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(``),
				out:      &map[string]any{},
			},
			want: want{
				data: &map[string]any{},
				err:  errors.New(`failed to decode response: EOF`),
			},
		},
		{
			name: "Invalid response data",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(`{"data":"invalid data format"}`),
				out:      &map[string]any{},
			},
			want: want{
				data: &map[string]any{},
				err:  errors.New(`failed to decode response data: json: cannot unmarshal JSON string into Go map[string]interface {}`),
			},
		},
		{
			// data が out の型と不一致でも、同梱された GraphQL errors をデコードエラーで握り潰さない
			name: "data が型不一致でも errors を優先して返す",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(`{"data":"invalid data format","errors":[{"message":"boom"}]}`),
				out:      &map[string]any{},
			},
			want: want{
				data: &map[string]any{},
				err:  gqlerror.List{{Message: "boom"}},
			},
		},
		{
			name: "Errors only response without data",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(`{"errors":[{"message":"Request error"}]}`),
				out:      &map[string]any{},
			},
			want: want{
				data: &map[string]any{},
				err:  gqlerror.List{{Message: "Request error"}},
			},
		},
		{
			name: "Empty object response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				respBody: []byte(`{}`),
				out:      &map[string]any{},
			},
			want: want{
				data: &map[string]any{},
				err:  errors.New(`failed to decode response: no data or errors member`),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := unmarshalResponse(tt.args.respBody, tt.args.out)

			// Error validation - compare error messages only
			if (err == nil) != (tt.want.err == nil) {
				t.Errorf("unmarshalResponse() error:\nwant: %v\n got: %v", tt.want.err, err)
			} else if err != nil && tt.want.err != nil {
				// Special handling for GraphQL errors
				var gqlErrs gqlerror.List
				var wantGqlErrs gqlerror.List
				if errors.As(err, &gqlErrs) && errors.As(tt.want.err, &wantGqlErrs) {
					// Compare GraphQL errors
					if !cmp.Equal(gqlErrs, wantGqlErrs) {
						t.Errorf("unmarshalResponse() GraphQL error:\nwant: %v\n got: %v", tt.want.err, err)
					}
				} else {
					// json/v2 は同義の "cannot unmarshal" と "unable to unmarshal" を
					// 非決定的に出し分けるため、表記を正規化してから比較する
					normalize := func(s string) string {
						return strings.ReplaceAll(s, "unable to unmarshal", "cannot unmarshal")
					}
					if normalize(tt.want.err.Error()) != normalize(err.Error()) {
						t.Errorf("unmarshalResponse() error message:\nwant: %v\n got: %v", tt.want.err.Error(), err.Error())
					}
				}
			}

			// Data comparison
			if !cmp.Equal(tt.want.data, tt.args.out, cmpopts.EquateEmpty()) {
				t.Errorf("unmarshalResponse() data:\nwant: %v\n got: %v", tt.want.data, tt.args.out)
			}
		})
	}
}

func TestClient_parseResponse(t *testing.T) {
	type testUser struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type fields struct {
		client   *http.Client
		endpoint string
	}

	type args struct {
		resp *http.Response
		out  any
	}

	type want struct {
		out any
		err error
	}

	tests := []struct {
		want   want
		args   args
		fields fields
		name   string
	}{
		{
			name: "Successful response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"data":{"user":{"id":"1","name":"John Doe"}}}`))),
					Header:     http.Header{},
				},
				out: &struct {
					User testUser `json:"user"`
				}{},
			},
			want: want{
				out: &struct {
					User testUser `json:"user"`
				}{
					User: testUser{
						ID:   "1",
						Name: "John Doe",
					},
				},
				err: nil,
			},
		},
		{
			name: "Gzipped response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				resp: gzipResponse(`{"data":{"user":{"id":"2","name":"Jane Doe"}}}`),
				out: &struct {
					User testUser `json:"user"`
				}{},
			},
			want: want{
				out: &struct {
					User testUser `json:"user"`
				}{
					User: testUser{
						ID:   "2",
						Name: "Jane Doe",
					},
				},
				err: nil,
			},
		},
		{
			name: "HTTP error status",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				resp: &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"message":"Internal Server Error"}`))),
					Header:     http.Header{},
				},
				out: &map[string]any{},
			},
			want: want{
				out: &map[string]any{},
				err: &ErrorResponse{
					NetworkError: &HTTPError{
						Code:    http.StatusInternalServerError,
						Message: `Response body {"message":"Internal Server Error"}`,
					},
				},
			},
		},
		{
			name: "GraphQL error in successful HTTP response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"errors":[{"message":"Field not found","path":["user"]}],"data":null}`))),
					Header:     http.Header{},
				},
				out: &map[string]any{},
			},
			want: want{
				out: &map[string]any{},
				err: &ErrorResponse{
					GqlErrors: &gqlerror.List{{Message: "Field not found", Path: ast.Path{ast.PathName("user")}}},
				},
			},
		},
		{
			name: "Invalid JSON in successful HTTP response",
			fields: fields{
				client:   &http.Client{},
				endpoint: "https://example.com/graphql",
			},
			args: args{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"data":invalid_json}`))),
					Header:     http.Header{},
				},
				out: &map[string]any{},
			},
			want: want{
				out: &map[string]any{},
				err: errors.New(`http status is OK but failed to decode response data: jsontext: invalid character 'i' at start of value within "/data" after offset 8`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseResponse(tt.args.resp, tt.args.out)

			// Error validation
			if (err == nil) != (tt.want.err == nil) {
				// Failure if only one is nil
				t.Errorf("parseResponse() error:\nwant: %v\n got: %v", tt.want.err, err)
			} else if err != nil {
				// For error responses, check error type
				var gotErrResp *ErrorResponse

				var wantErrResp *ErrorResponse

				// Check if errors are ErrorResponse type
				if errors.As(err, &gotErrResp) && errors.As(tt.want.err, &wantErrResp) {
					// Check network error
					if (gotErrResp.NetworkError == nil) != (wantErrResp.NetworkError == nil) {
						t.Errorf("parseResponse() network error existence mismatch:\nwant: %v\n got: %v",
							wantErrResp.NetworkError != nil, gotErrResp.NetworkError != nil)
					} else if gotErrResp.NetworkError != nil && wantErrResp.NetworkError != nil {
						// Compare network error attributes
						if gotErrResp.NetworkError.Code != wantErrResp.NetworkError.Code ||
							gotErrResp.NetworkError.Message != wantErrResp.NetworkError.Message {
							t.Errorf("parseResponse() network error mismatch:\nwant: %v\n got: %v",
								wantErrResp.NetworkError, gotErrResp.NetworkError)
						}
					}

					// Check GraphQL errors
					if (gotErrResp.GqlErrors == nil) != (wantErrResp.GqlErrors == nil) {
						t.Errorf("parseResponse() GraphQL error existence mismatch:\nwant: %v\n got: %v",
							wantErrResp.GqlErrors != nil, gotErrResp.GqlErrors != nil)
					} else if gotErrResp.GqlErrors != nil && wantErrResp.GqlErrors != nil {
						// Compare GraphQL error messages
						if len(*gotErrResp.GqlErrors) != len(*wantErrResp.GqlErrors) {
							t.Errorf("parseResponse() GraphQL error count mismatch:\nwant: %v\n got: %v",
								len(*wantErrResp.GqlErrors), len(*gotErrResp.GqlErrors))
						}
					}
				} else if !cmputil.EqualErrorMessage(tt.want.err, err) {
					t.Errorf("parseResponse() error message:\nwant: %v\n got: %v", tt.want.err, err)
				}
			}

			// Data comparison for non-error cases
			if tt.want.err == nil {
				if !cmp.Equal(tt.want.out, tt.args.out, cmpopts.EquateEmpty()) {
					t.Errorf("parseResponse() output:\nwant: %v\n got: %v", tt.want.out, tt.args.out)
				}
			}
		})
	}
}

// Helper function to create a gzipped HTTP response.
func gzipResponse(jsonBody string) *http.Response {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, _ = gzWriter.Write([]byte(jsonBody))
	gzWriter.Close()

	header := http.Header{}
	header.Set("Content-Encoding", "gzip")

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&buf),
		Header:     header,
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTruncateForMessage(t *testing.T) {
	t.Parallel()

	type args struct {
		s string
	}

	type want struct {
		message string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 上限以下のボディはそのまま返す
			name: "上限以下はそのまま返す",
			args: args{
				s: "short body",
			},
			want: want{
				message: "short body",
			},
		},
		{
			// 上限超過の ASCII ボディは上限で切り詰め、全長の注記を付ける
			name: "上限超過のASCIIボディは切り詰めて注記を付ける",
			args: args{
				s: strings.Repeat("a", maxErrorBodyLen+10),
			},
			want: want{
				message: strings.Repeat("a", maxErrorBodyLen) + "… (1034 bytes total, truncated; full body in HTTPError.Body)",
			},
		},
		{
			// 切り詰め位置がマルチバイト文字の途中になる場合は手前のルーン境界まで戻す
			name: "切り詰め位置がマルチバイト境界をまたぐ場合は手前のルーン境界で切る",
			args: args{
				s: strings.Repeat("a", maxErrorBodyLen-2) + "ああ",
			},
			want: want{
				message: strings.Repeat("a", maxErrorBodyLen-2) + "… (1028 bytes total, truncated; full body in HTTPError.Body)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateForMessage(tt.args.s)

			if diff := cmp.Diff(tt.want.message, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
