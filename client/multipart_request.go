package client

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"reflect"
	"strconv"
	"strings"

	"github.com/99designs/gqlgen/graphql"
)

var (
	// errUploadWithoutFile は、File が nil の graphql.Upload を送ろうとしたことを表す。
	// 必須の Upload! 変数は値型で生成されるため、設定し忘れたゼロ値で起こり得る。
	errUploadWithoutFile = errors.New("graphql.Upload has a nil File")
	// errUploadContentTypeLineBreak は、graphql.Upload の ContentType が改行を含むことを表す。
	// そのままパートのヘッダーに書くと、ヘッダーと本文の区切りが壊れる。
	errUploadContentTypeLineBreak = errors.New("graphql.Upload ContentType contains a line break")
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
// (jsontext.Encoder の StackPointer) ごと収集される。nullable な変数・input フィールドに
// 生成される graphql.Omittable[T] の中の Upload も収集する。
func (c *uploadCollector) marshalers() *json.Marshalers {
	return json.JoinMarshalers(
		json.MarshalToFunc(func(enc *jsontext.Encoder, upload graphql.Upload) error {
			return c.collect(enc, upload)
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, upload *graphql.Upload) error {
			if upload == nil {
				if err := enc.WriteToken(jsontext.Null); err != nil {
					return fmt.Errorf("write null: %w", err)
				}
				return nil
			}
			return c.collect(enc, *upload)
		}),
		json.MarshalToFunc(marshalOmittable),
	)
}

// collect は Upload の位置に JSON null を書き、ファイルとその位置を記録する。位置は JSON Pointer
// (例: /variables/input/file) を graphql-multipart-request-spec のパス表記 (例: variables.input.file)
// に変換したもの。File が nil の Upload と、ContentType が改行を含む Upload はエラーにする。
func (c *uploadCollector) collect(enc *jsontext.Encoder, upload graphql.Upload) error {
	if upload.File == nil {
		return errUploadWithoutFile
	}
	if strings.ContainsAny(upload.ContentType, "\r\n") {
		return fmt.Errorf("%w: %q", errUploadContentTypeLineBreak, upload.ContentType)
	}

	if err := enc.WriteToken(jsontext.Null); err != nil {
		return fmt.Errorf("write null: %w", err)
	}

	c.files = append(c.files, upload)
	c.paths = append(c.paths, strings.ReplaceAll(strings.TrimPrefix(string(enc.StackPointer()), "/"), "/", "."))

	return nil
}

// omittable は graphql.Omittable[T] を T によらず捕捉するためのインターフェース。
// 型パラメータを含まないメソッドだけを並べ、Omittable かどうかは marshalOmittable が型で確かめる。
type omittable interface {
	IsSet() bool
}

// omittablePkgPath と omittableTypePrefix は graphql.Omittable[T] の型を見分けるための
// パッケージのパスと型の名前の接頭辞。
const (
	omittablePkgPath    = "github.com/99designs/gqlgen/graphql"
	omittableTypePrefix = "Omittable["
)

// marshalOmittable は graphql.Omittable[T] の中の値を、呼び出し元と同じオプション (Upload の
// マーシャラを含む) でエンコードする。Omittable は encoding/json (v1) の MarshalJSON しか持たず、
// そのまま任せると中の Upload に json/v2 のマーシャラが届かず、ファイルのパートにならない。
// 値の設定されていない Omittable は、Omittable.MarshalJSON と同じく null にする。
// graphql.Omittable[T] でない型は errors.ErrUnsupported を返し、通常のエンコードに任せる。
func marshalOmittable(enc *jsontext.Encoder, o omittable) error {
	// インターフェースの型のマーシャラには、json/v2 が常に値へのポインタを渡す。
	value := reflect.Indirect(reflect.ValueOf(o))
	if value.Type().PkgPath() != omittablePkgPath || !strings.HasPrefix(value.Type().Name(), omittableTypePrefix) {
		return errors.ErrUnsupported
	}

	if !o.IsSet() {
		if err := enc.WriteToken(jsontext.Null); err != nil {
			return fmt.Errorf("write null: %w", err)
		}
		return nil
	}

	inner := value.MethodByName("Value").Call(nil)[0].Interface()
	if err := json.MarshalEncode(enc, inner); err != nil {
		return fmt.Errorf("encode omittable value: %w", err)
	}

	return nil
}

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
// Content-Disposition は mime/multipart の CreateFormFile と同じく、" と \ を \ でエスケープし、
// CR と LF を %0D・%0A にする (ファイル名の改行でヘッダーを足されないようにする)。
func filePartHeader(fieldName string, upload graphql.Upload) textproto.MIMEHeader {
	contentType := upload.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", multipart.FileContentDisposition(fieldName, upload.Filename))
	header.Set("Content-Type", contentType)

	return header
}
