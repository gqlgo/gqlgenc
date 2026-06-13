package client

import (
	json "encoding/json/v2"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/99designs/gqlgen/graphql"
)

// newUpload はテスト用の一時ファイルを作成して graphql.Upload を返す。
func newUpload(t *testing.T, name, content string) graphql.Upload {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })

	return graphql.Upload{
		File:     file,
		Filename: name,
		Size:     int64(len(content)),
	}
}

func TestNewRequestUpload(t *testing.T) {
	type args struct {
		operationName string
		query         string
		variables     func(t *testing.T) any
	}

	type want struct {
		multipart          bool
		operationsContains []string
		mapping            map[string][]string
		files              map[string]string
		bodyContains       []string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// 単一ファイルは multipart/form-data として送信される
			name: "単一ファイルのアップロード",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload!) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					return map[string]any{
						"file": newUpload(t, "test.txt", "test content"),
					}
				},
			},
			want: want{
				multipart:          true,
				operationsContains: []string{`"file":null`},
				mapping: map[string][]string{
					"0": {"variables.file"},
				},
				files: map[string]string{
					"0": "test content",
				},
			},
		},
		{
			// 型付き構造体の variables でも、ネストした input 内の Upload を検出できる
			name: "型付き構造体のネストしたinput内のアップロード",
			args: args{
				operationName: "UploadAvatar",
				query:         "mutation UploadAvatar($input: AvatarInput!) { uploadAvatar(input: $input) }",
				variables: func(t *testing.T) any {
					t.Helper()
					type avatarInput struct {
						Name string          `json:"name"`
						File *graphql.Upload `json:"file"`
					}
					type vars struct {
						Input avatarInput `json:"input"`
					}
					upload := newUpload(t, "avatar.png", "image bytes")
					return vars{
						Input: avatarInput{
							Name: "icon",
							File: &upload,
						},
					}
				},
			},
			want: want{
				multipart:          true,
				operationsContains: []string{`"name":"icon"`, `"file":null`},
				mapping: map[string][]string{
					"0": {"variables.input.file"},
				},
				files: map[string]string{
					"0": "image bytes",
				},
			},
		},
		{
			// 構造体の配列の中にネストした Upload も収集できる (gqlgo/gqlgenc#292)。
			// トップレベルの Upload (preview) と、配列要素ごとの Upload (images.N.image) が混在する。
			name: "構造体の配列内にネストしたアップロード",
			args: args{
				operationName: "CreateProduct",
				query:         "mutation CreateProduct($input: NewProductInput!) { createProduct(input: $input) }",
				variables: func(t *testing.T) any {
					t.Helper()
					type productImageInput struct {
						Order int             `json:"order"`
						Image *graphql.Upload `json:"image"`
					}
					type newProductInput struct {
						Title   string               `json:"title"`
						Preview *graphql.Upload      `json:"preview"`
						Images  []*productImageInput `json:"images"`
					}
					type vars struct {
						Input newProductInput `json:"input"`
					}
					preview := newUpload(t, "preview.png", "preview bytes")
					first := newUpload(t, "first.png", "first image")
					second := newUpload(t, "second.png", "second image")
					return vars{
						Input: newProductInput{
							Title:   "Product",
							Preview: &preview,
							Images: []*productImageInput{
								{Order: 1, Image: &first},
								{Order: 2, Image: &second},
							},
						},
					}
				},
			},
			want: want{
				multipart:          true,
				operationsContains: []string{`"title":"Product"`, `"preview":null`, `"image":null`, `"order":1`, `"order":2`},
				mapping: map[string][]string{
					"0": {"variables.input.preview"},
					"1": {"variables.input.images.0.image"},
					"2": {"variables.input.images.1.image"},
				},
				files: map[string]string{
					"0": "preview bytes",
					"1": "first image",
					"2": "second image",
				},
			},
		},
		{
			// 複数ファイルはリストの要素ごとに収集される
			name: "複数ファイルのアップロード",
			args: args{
				operationName: "UploadFiles",
				query:         "mutation UploadFiles($files: [Upload!]!) { uploadFiles(files: $files) }",
				variables: func(t *testing.T) any {
					t.Helper()
					first := newUpload(t, "test.txt", "test content")
					second := newUpload(t, "test2.txt", "another test content")
					return map[string]any{
						"files": []*graphql.Upload{&first, &second},
					}
				},
			},
			want: want{
				multipart:          true,
				operationsContains: []string{`"files":[null,null]`},
				mapping: map[string][]string{
					"0": {"variables.files.0"},
					"1": {"variables.files.1"},
				},
				files: map[string]string{
					"0": "test content",
					"1": "another test content",
				},
			},
		},
		{
			// Upload を含まない variables は通常の JSON リクエストになる
			name: "Uploadなしの場合はJSONリクエスト",
			args: args{
				operationName: "TestQuery",
				query:         "query TestQuery { test }",
				variables: func(t *testing.T) any {
					t.Helper()
					return map[string]any{}
				},
			},
			want: want{
				multipart:    false,
				bodyContains: []string{`"operationName":"TestQuery"`},
			},
		},
		{
			// nil の *graphql.Upload は null として送信され multipart にはならない
			name: "nilのUploadポインタはJSONリクエスト",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					return map[string]any{
						"file": (*graphql.Upload)(nil),
					}
				},
			},
			want: want{
				multipart:    false,
				bodyContains: []string{`"file":null`},
			},
		},
		{
			// 空のファイルリストは multipart にはならない
			name: "空のファイルリストはJSONリクエスト",
			args: args{
				operationName: "UploadFiles",
				query:         "mutation UploadFiles($files: [Upload!]) { uploadFiles(files: $files) }",
				variables: func(t *testing.T) any {
					t.Helper()
					return map[string]any{
						"files": []*graphql.Upload{},
					}
				},
			},
			want: want{
				multipart:    false,
				bodyContains: []string{`"files":[]`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRequest(t.Context(), "http://example.com/graphql", tt.args.operationName, tt.args.query, tt.args.variables(t))
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}

			if !tt.want.multipart {
				if ct := got.Header.Get("Content-Type"); ct != "application/json;charset=utf-8" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}

				body, err := io.ReadAll(got.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}

				for _, contains := range tt.want.bodyContains {
					if !strings.Contains(string(body), contains) {
						t.Errorf("body does not contain %q: %s", contains, body)
					}
				}

				return
			}

			if ct := got.Header.Get("Content-Type"); !strings.Contains(ct, "multipart/form-data") {
				t.Errorf("Content-Type = %q, want to contain multipart/form-data", ct)
			}

			if err := got.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm() error = %v", err)
			}

			// operations フィールドの検証
			operations := got.MultipartForm.Value["operations"]
			if len(operations) != 1 {
				t.Fatalf("operations field count = %d, want 1", len(operations))
			}
			for _, contains := range tt.want.operationsContains {
				if !strings.Contains(operations[0], contains) {
					t.Errorf("operations does not contain %q: %s", contains, operations[0])
				}
			}

			// map フィールドの検証
			mapValues := got.MultipartForm.Value["map"]
			if len(mapValues) != 1 {
				t.Fatalf("map field count = %d, want 1", len(mapValues))
			}
			var mapping map[string][]string
			if err := json.Unmarshal([]byte(mapValues[0]), &mapping); err != nil {
				t.Fatalf("failed to unmarshal map field: %v", err)
			}
			if diff := cmp.Diff(tt.want.mapping, mapping); diff != "" {
				t.Errorf("map field diff(-want +got): %s", diff)
			}

			// ファイルパートの検証
			if len(got.MultipartForm.File) != len(tt.want.files) {
				t.Errorf("file count = %d, want %d", len(got.MultipartForm.File), len(tt.want.files))
			}
			for fieldName, wantContent := range tt.want.files {
				headers := got.MultipartForm.File[fieldName]
				if len(headers) != 1 {
					t.Errorf("file field %q count = %d, want 1", fieldName, len(headers))
					continue
				}

				file, err := headers[0].Open()
				if err != nil {
					t.Fatalf("failed to open file part %q: %v", fieldName, err)
				}
				content, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					t.Fatalf("failed to read file part %q: %v", fieldName, err)
				}

				if diff := cmp.Diff(wantContent, string(content)); diff != "" {
					t.Errorf("file %q content diff(-want +got): %s", fieldName, diff)
				}
			}
		})
	}
}
