package client

import (
	json "encoding/json/v2"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

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

func TestNewPostRequestUpload(t *testing.T) {
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
		partHeaders        map[string]textproto.MIMEHeader
		bodyContains       []string
		err                error
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
		{
			// nullable な Upload の変数に生成される graphql.Omittable[*graphql.Upload] の中の Upload も
			// ファイルのパートとして送信される。Omittable は v1 の MarshalJSON しか持たないため、
			// 中の値をマーシャラに通さないと JSON の {"File":{},...} として送られる。
			name: "Omittableに包んだUploadポインタのアップロード",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					type vars struct {
						File graphql.Omittable[*graphql.Upload] `json:"file,omitzero"`
					}
					upload := newUpload(t, "test.txt", "test content")
					return vars{File: graphql.OmittableOf(&upload)}
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
			// ネストした input の Omittable に包んだ Upload の値も、その位置ごと収集される
			name: "ネストしたinputのOmittableに包んだUploadのアップロード",
			args: args{
				operationName: "UploadAvatar",
				query:         "mutation UploadAvatar($input: AvatarInput!) { uploadAvatar(input: $input) }",
				variables: func(t *testing.T) any {
					t.Helper()
					type avatarInput struct {
						Name graphql.Omittable[*string]        `json:"name,omitzero"`
						File graphql.Omittable[graphql.Upload] `json:"file,omitzero"`
					}
					type vars struct {
						Input avatarInput `json:"input"`
					}
					name := "icon"
					return vars{
						Input: avatarInput{
							Name: graphql.OmittableOf(&name),
							File: graphql.OmittableOf(newUpload(t, "avatar.png", "image bytes")),
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
			// nil を設定した Omittable は null として送信され multipart にはならない
			name: "nilを設定したOmittableのUploadはJSONリクエスト",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					type vars struct {
						File graphql.Omittable[*graphql.Upload] `json:"file,omitzero"`
					}
					return vars{File: graphql.OmittableOf[*graphql.Upload](nil)}
				},
			},
			want: want{
				multipart:    false,
				bodyContains: []string{`"variables":{"file":null}`},
			},
		},
		{
			// 値を設定していない Omittable は omitzero で省略され multipart にはならない。
			// variables が空のオブジェクトになるため、variables ごと省略される。
			name: "値を設定していないOmittableのUploadは省略する",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					type vars struct {
						File graphql.Omittable[*graphql.Upload] `json:"file,omitzero"`
					}
					return vars{}
				},
			},
			want: want{
				multipart:    false,
				bodyContains: []string{`"operationName":"UploadFile"}`},
			},
		},
		{
			// File が nil の Upload (必須の Upload! 変数の設定し忘れ) はエラーにする。
			// 本文を書く goroutine で nil の File を読むと、プロセスごと panic で落ちる。
			name: "FileがnilのUploadはエラー",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload!) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					type vars struct {
						File graphql.Upload `json:"file"`
					}
					return vars{}
				},
			},
			want: want{
				err: errUploadWithoutFile,
			},
		},
		{
			// ContentType の改行はパートのヘッダーと本文の区切りを壊すため、エラーにする
			name: "ContentTypeに改行を含むUploadはエラー",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload!) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					upload := newUpload(t, "test.txt", "test content")
					upload.ContentType = "text/plain\r\nX-Injected: 1"
					return map[string]any{"file": upload}
				},
			},
			want: want{
				err: errUploadContentTypeLineBreak,
			},
		},
		{
			// ファイル名の " と \ はエスケープし、CR と LF は %0D・%0A にする (mime/multipart の
			// CreateFormFile と同じ)。改行をそのまま書くと、受け側がファイル名の後ろをパートの
			// ヘッダー (X-Injected) と本文として読む。
			name: "ファイル名の引用符と改行はエスケープする",
			args: args{
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload!) { uploadFile(file: $file) }",
				variables: func(t *testing.T) any {
					t.Helper()
					upload := newUpload(t, "test.txt", "test content")
					upload.Filename = "a.txt\"\r\nX-Injected: 1\r\n\r\nbody"
					upload.ContentType = "text/plain"
					return map[string]any{"file": upload}
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
				partHeaders: map[string]textproto.MIMEHeader{
					"0": {
						"Content-Disposition": {`form-data; name="0"; filename="a.txt\"%0D%0AX-Injected: 1%0D%0A%0D%0Abody"`},
						"Content-Type":        {"text/plain"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newPostRequest(t.Context(), "http://example.com/graphql", tt.args.operationName, tt.args.query, tt.args.variables(t))
			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("newPostRequest() error diff(-want +got): %s", diff)
			}
			if tt.want.err != nil {
				return
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

				if wantHeader, ok := tt.want.partHeaders[fieldName]; ok {
					if diff := cmp.Diff(wantHeader, headers[0].Header); diff != "" {
						t.Errorf("file %q header diff(-want +got): %s", fieldName, diff)
					}
				}
			}
		})
	}
}
