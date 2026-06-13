package client

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
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

// newMultipartRequest は graphql-multipart-request-spec に従った
// multipart/form-data リクエストを構築する。
// https://github.com/jaydenseric/graphql-multipart-request-spec
// https://gqlgen.com/reference/file-upload/
func newMultipartRequest(ctx context.Context, endpoint string, operations []byte, uploads *uploadCollector) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	operationsWriter, err := writer.CreateFormField("operations")
	if err != nil {
		return nil, fmt.Errorf("create form field operations: %w", err)
	}

	if _, err := operationsWriter.Write(operations); err != nil {
		return nil, fmt.Errorf("write operations: %w", err)
	}

	mapping := make(map[string][]string, len(uploads.paths))
	for i, path := range uploads.paths {
		mapping[strconv.Itoa(i)] = []string{path}
	}

	mapWriter, err := writer.CreateFormField("map")
	if err != nil {
		return nil, fmt.Errorf("create form field map: %w", err)
	}

	if err := json.MarshalWrite(mapWriter, mapping); err != nil {
		return nil, fmt.Errorf("encode map: %w", err)
	}

	for i, upload := range uploads.files {
		part, err := writer.CreateFormFile(strconv.Itoa(i), upload.Filename)
		if err != nil {
			return nil, fmt.Errorf("form file %w", err)
		}

		if _, err := io.Copy(part, upload.File); err != nil {
			return nil, fmt.Errorf("copy file %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("writer close %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create request struct failed: %w", err)
	}

	req.Header = http.Header{
		"Content-Type": []string{writer.FormDataContentType()},
		"Accept":       []string{acceptGraphQLResponse, contentTypeJSON},
	}

	return req, nil
}
