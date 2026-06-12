package client

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestParseResponse(t *testing.T) {
	type testData struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		gzipResponse   bool
		expectedOutput *testData
		expectError    bool
		errorContains  string
	}{
		{
			name:           "Normal response",
			statusCode:     http.StatusOK,
			responseBody:   `{"data":{"name":"Test"}}`,
			expectedOutput: &testData{Name: "Test"},
			expectError:    false,
		},
		{
			name:           "Gzip compressed response",
			statusCode:     http.StatusOK,
			responseBody:   `{"data":{"name":"Compressed Test"}}`,
			gzipResponse:   true,
			expectedOutput: &testData{Name: "Compressed Test"},
			expectError:    false,
		},
		{
			name:          "HTTP error response",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"message":"Server Error"}`,
			expectError:   true,
			errorContains: "networkErrors",
		},
		{
			name:          "GraphQL error response",
			statusCode:    http.StatusOK,
			responseBody:  `{"data":null,"errors":[{"message":"GraphQL Error"}]}`,
			expectError:   true,
			errorContains: "graphqlErrors",
		},
		{
			name:          "Invalid JSON response",
			statusCode:    http.StatusOK,
			responseBody:  `Invalid JSON`,
			expectError:   true,
			errorContains: "failed to decode response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader = bytes.NewBufferString(tt.responseBody)
			// gzipで圧縮する場合
			if tt.gzipResponse {
				var buf bytes.Buffer
				gzipWriter := gzip.NewWriter(&buf)
				_, err := gzipWriter.Write([]byte(tt.responseBody))
				if err != nil {
					t.Fatalf("Failed to write to gzip writer: %v", err)
				}
				if err := gzipWriter.Close(); err != nil {
					t.Fatalf("Failed to close gzip writer: %v", err)
				}
				body = &buf
			}

			// HTTPレスポンスを作成
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(body),
				Header:     make(http.Header),
			}

			if tt.gzipResponse {
				resp.Header.Set("Content-Encoding", "gzip")
			}

			// テスト対象の関数を実行
			var result testData
			err := ParseResponse(resp, &result)

			// 期待する結果を検証
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, but got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result.Name != tt.expectedOutput.Name {
					t.Errorf("Expected Name to be %q, but got %q", tt.expectedOutput.Name, result.Name)
				}
			}
		})
	}
}

func TestUnmarshalResponse(t *testing.T) {
	type testData struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name           string
		responseBody   []byte
		expectedOutput *testData
		expectError    bool
		errorContains  string
	}{
		{
			name:           "Normal response",
			responseBody:   []byte(`{"data":{"name":"Test"}}`),
			expectedOutput: &testData{Name: "Test"},
			expectError:    false,
		},
		{
			name:          "GraphQL error response",
			responseBody:  []byte(`{"data":null,"errors":[{"message":"GraphQL Error"}]}`),
			expectError:   true,
			errorContains: "GraphQL Error",
		},
		{
			name:          "Invalid JSON response",
			responseBody:  []byte(`Invalid JSON`),
			expectError:   true,
			errorContains: "failed to decode response",
		},
		{
			name:          "Invalid data format",
			responseBody:  []byte(`{"data":"Invalid data format"}`),
			expectError:   true,
			errorContains: "failed to decode response data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result testData
			err := unmarshalResponse(tt.responseBody, &result)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, but got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result.Name != tt.expectedOutput.Name {
					t.Errorf("Expected Name to be %q, but got %q", tt.expectedOutput.Name, result.Name)
				}
			}
		})
	}
}

func TestErrorResponse_HasErrors(t *testing.T) {
	tests := []struct {
		name     string
		response ErrorResponse
		want     bool
	}{
		{
			name:     "No errors",
			response: ErrorResponse{},
			want:     false,
		},
		{
			name: "Network error present",
			response: ErrorResponse{
				NetworkError: &HTTPError{
					Code:    500,
					Message: "Server Error",
				},
			},
			want: true,
		},
		{
			name: "GraphQL error present",
			response: ErrorResponse{
				GqlErrors: &gqlerror.List{
					&gqlerror.Error{
						Message: "GraphQL Error",
					},
				},
			},
			want: true,
		},
		{
			name: "Both errors present",
			response: ErrorResponse{
				NetworkError: &HTTPError{
					Code:    500,
					Message: "Server Error",
				},
				GqlErrors: &gqlerror.List{
					&gqlerror.Error{
						Message: "GraphQL Error",
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.HasErrors()
			if got != tt.want {
				t.Errorf("HasErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorResponse_Error(t *testing.T) {
	tests := []struct {
		name     string
		response ErrorResponse
		contains []string
	}{
		{
			name: "Network error",
			response: ErrorResponse{
				NetworkError: &HTTPError{
					Code:    500,
					Message: "Server Error",
				},
			},
			contains: []string{"networkErrors", "Server Error", "500"},
		},
		{
			name: "GraphQL error",
			response: ErrorResponse{
				GqlErrors: &gqlerror.List{
					&gqlerror.Error{
						Message: "GraphQL Error",
					},
				},
			},
			contains: []string{"graphqlErrors", "GraphQL Error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.response.Error()
			for _, s := range tt.contains {
				if !strings.Contains(errStr, s) {
					t.Errorf("Error() result should contain %q, but got %q", s, errStr)
				}
			}
		})
	}
}

func TestErrorResponse_Unwrap(t *testing.T) {
	type fields struct {
		response *ErrorResponse
	}

	type want struct {
		httpError bool
		gqlErrors bool
	}

	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			name: "NetworkErrorはerrors.Asで*HTTPErrorとして取り出せる",
			fields: fields{
				response: &ErrorResponse{
					NetworkError: &HTTPError{Code: 500, Message: "Server Error"},
				},
			},
			want: want{
				httpError: true,
			},
		},
		{
			name: "GqlErrorsはerrors.Asでgqlerror.Listとして取り出せる",
			fields: fields{
				response: &ErrorResponse{
					GqlErrors: &gqlerror.List{&gqlerror.Error{Message: "GraphQL Error"}},
				},
			},
			want: want{
				gqlErrors: true,
			},
		},
		{
			name: "両方あるときは両方取り出せる",
			fields: fields{
				response: &ErrorResponse{
					NetworkError: &HTTPError{Code: 500, Message: "Server Error"},
					GqlErrors:    &gqlerror.List{&gqlerror.Error{Message: "GraphQL Error"}},
				},
			},
			want: want{
				httpError: true,
				gqlErrors: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := error(tt.fields.response)

			var httpErr *HTTPError
			if got := errors.As(err, &httpErr); got != tt.want.httpError {
				t.Errorf("errors.As(*HTTPError) = %v, want %v", got, tt.want.httpError)
			}

			var gqlErrs gqlerror.List
			if got := errors.As(err, &gqlErrs); got != tt.want.gqlErrors {
				t.Errorf("errors.As(gqlerror.List) = %v, want %v", got, tt.want.gqlErrors)
			}
		})
	}
}
