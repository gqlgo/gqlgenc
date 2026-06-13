package client

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/99designs/gqlgen/graphql"
)

// uploadCollector は variables のエンコード中に現れた graphql.Upload を収集する。
// 値の位置には JSON null を書き込み、ファイル本体とその JSON パスを
// multipart リクエストの構築のために記録する。
type uploadCollector struct {
	files []graphql.Upload
	paths []string
}

// marshalers は graphql.Upload と *graphql.Upload を捕捉する json/v2 のマーシャラを返す。
// ネストした input オブジェクトやリスト内の Upload も、エンコード中に現れた位置
// (jsontext.Encoder の StackPointer) ごと収集される。
func (c *uploadCollector) marshalers() *json.Marshalers {
	return json.JoinMarshalers(
		json.MarshalToFunc(func(enc *jsontext.Encoder, upload graphql.Upload) error {
			if err := enc.WriteToken(jsontext.Null); err != nil {
				return fmt.Errorf("write null: %w", err)
			}
			c.collect(upload, enc.StackPointer())
			return nil
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, upload *graphql.Upload) error {
			if upload == nil {
				if err := enc.WriteToken(jsontext.Null); err != nil {
					return fmt.Errorf("write null: %w", err)
				}
				return nil
			}
			if err := enc.WriteToken(jsontext.Null); err != nil {
				return fmt.Errorf("write null: %w", err)
			}
			c.collect(*upload, enc.StackPointer())
			return nil
		}),
	)
}

// collect は JSON Pointer (例: /variables/input/file) を
// graphql-multipart-request-spec のパス表記 (例: variables.input.file) に変換して記録する。
func (c *uploadCollector) collect(upload graphql.Upload, pointer jsontext.Pointer) {
	c.files = append(c.files, upload)
	c.paths = append(c.paths, strings.ReplaceAll(strings.TrimPrefix(string(pointer), "/"), "/", "."))
}

// quoteEscaper は MIME ヘッダーのクォート内で特別扱いされる文字をエスケープする。
// mime/multipart 本体が form フィールドに対して行うエスケープと同じ規則。
var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// newMultipartRequest は graphql-multipart-request-spec に従った
// multipart/form-data リクエストを構築する。ボディは io.Pipe 経由でストリーミングし、
// ファイル本体をメモリに溜め込まずに送信する。書き込み中のエラーは io.Pipe を通じて
// 送信側に伝播し、リクエストはそのエラーで失敗する。
// https://github.com/jaydenseric/graphql-multipart-request-spec
// https://gqlgen.com/reference/file-upload/
func newMultipartRequest(ctx context.Context, endpoint string, operations []byte, uploads *uploadCollector) (*http.Request, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	contentType := writer.FormDataContentType()

	go func() {
		if err := writeMultipartBody(writer, operations, uploads); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if err := writer.Close(); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("close multipart writer: %w", err))
			return
		}
		pw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		return nil, fmt.Errorf("create request struct failed: %w", err)
	}

	req.Header = http.Header{
		headerContentType: []string{contentType},
		headerAccept:      acceptHeaderValues(),
	}

	return req, nil
}

// writeMultipartBody は operations / map / 各ファイルパートを multipart writer に書き込む。
func writeMultipartBody(writer *multipart.Writer, operations []byte, uploads *uploadCollector) error {
	operationsWriter, err := writer.CreateFormField("operations")
	if err != nil {
		return fmt.Errorf("create form field operations: %w", err)
	}
	if _, err := operationsWriter.Write(operations); err != nil {
		return fmt.Errorf("write operations: %w", err)
	}

	mapping := make(map[string][]string, len(uploads.paths))
	for i, path := range uploads.paths {
		mapping[strconv.Itoa(i)] = []string{path}
	}

	mapWriter, err := writer.CreateFormField("map")
	if err != nil {
		return fmt.Errorf("create form field map: %w", err)
	}
	if err := json.MarshalWrite(mapWriter, mapping); err != nil {
		return fmt.Errorf("encode map: %w", err)
	}

	for i, upload := range uploads.files {
		part, err := writer.CreatePart(filePartHeader(strconv.Itoa(i), upload))
		if err != nil {
			return fmt.Errorf("create form file %d: %w", i, err)
		}
		if _, err := io.Copy(part, upload.File); err != nil {
			return fmt.Errorf("copy file %d: %w", i, err)
		}
	}

	return nil
}

// filePartHeader はファイルパートの MIME ヘッダーを組み立てる。Upload.ContentType が
// 指定されていればそれを使い、無ければ application/octet-stream にフォールバックする。
// Content-Disposition のエスケープは mime/multipart の CreateFormFile と同じにする。
func filePartHeader(fieldName string, upload graphql.Upload) textproto.MIMEHeader {
	contentType := upload.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(
		`form-data; name="%s"; filename="%s"`,
		quoteEscaper.Replace(fieldName), quoteEscaper.Replace(upload.Filename),
	))
	header.Set("Content-Type", contentType)

	return header
}
