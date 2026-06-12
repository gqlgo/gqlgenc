package client

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

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
				err:  errors.New(`failed to decode response data: json: cannot unmarshal JSON string into Go map[string]interface {} within "/data"`),
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
					// Compare error messages for other errors
					if tt.want.err.Error() != err.Error() {
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
			err := ParseResponse(tt.args.resp, tt.args.out)

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
				} else if tt.want.err.Error() != err.Error() {
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

func TestClient_Post(t *testing.T) {
	t.Parallel()

	type fields struct {
		options []Option
	}

	type args struct {
		operationName string
		query         string
		variables     map[string]any
	}

	type want struct {
		header http.Header
		err    error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "WithHTTPHeaderで設定したヘッダーがリクエストに付与される",
			fields: fields{
				options: []Option{
					WithHTTPHeader(http.Header{
						"Authorization": []string{"Bearer token"},
						"x-request-id":  []string{"req-1"},
					}),
				},
			},
			args: args{
				operationName: "GetUser",
				query:         "query GetUser { user { id } }",
			},
			want: want{
				header: http.Header{
					"Content-Type":  []string{"application/json;charset=utf-8"},
					"Accept":        []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
					"Authorization": []string{"Bearer token"},
					"X-Request-Id":  []string{"req-1"},
				},
			},
		},
		{
			name: "WithHTTPHeaderでデフォルトのContent-Typeをキー単位で上書きできる",
			fields: fields{
				options: []Option{
					WithHTTPHeader(http.Header{
						"Content-Type": []string{"application/json"},
					}),
				},
			},
			args: args{
				operationName: "GetUser",
				query:         "query GetUser { user { id } }",
			},
			want: want{
				header: http.Header{
					"Content-Type": []string{"application/json"},
					"Accept":       []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
				},
			},
		},
		{
			name: "ヘッダー未設定のときはデフォルトヘッダーのみが送信される",
			fields: fields{
				options: nil,
			},
			args: args{
				operationName: "GetUser",
				query:         "query GetUser { user { id } }",
			},
			want: want{
				header: http.Header{
					"Content-Type": []string{"application/json;charset=utf-8"},
					"Accept":       []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var gotHeader http.Header
			httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				gotHeader = req.Header
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"data":{}}`))),
					Header:     http.Header{},
				}, nil
			})}

			options := append([]Option{WithHTTPClient(httpClient)}, tt.fields.options...)
			c := NewClient("https://example.com/graphql", options...)

			var out map[string]any
			err := c.Post(t.Context(), tt.args.operationName, tt.args.query, tt.args.variables, &out)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.header, gotHeader); diff != "" {
				t.Errorf("header diff(-want +got): %s", diff)
			}
		})
	}
}
