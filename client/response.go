package client

import (
	"bytes"
	"compress/gzip"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

func parseResponse(resp *http.Response, out any) error {
	reader := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to decode gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	isStatusCodeOK := http.StatusOK <= resp.StatusCode && resp.StatusCode < http.StatusMultipleChoices

	var gqlErrors gqlerror.List
	if err := unmarshalResponse(body, out); err != nil {
		errs, ok := errors.AsType[gqlerror.List](err)
		if !ok && isStatusCodeOK {
			// status code is OK but the GraphQL response can't be parsed, it's an error.
			return fmt.Errorf("http status is OK but %w", err)
		}
		// status code is not OK and the body is not a GraphQL error response:
		// keep the network error only (errs is nil here).
		gqlErrors = errs
	}

	// status OK and no GraphQL errors: success, no allocation.
	if isStatusCodeOK && gqlErrors == nil {
		return nil
	}

	errResponse := &ErrorResponse{}
	if !isStatusCodeOK {
		bodyStr := string(body)
		errResponse.NetworkError = &HTTPError{
			Code:    resp.StatusCode,
			Message: "Response body " + truncateForMessage(bodyStr),
			Body:    bodyStr,
		}
	}
	if gqlErrors != nil {
		errResponse.GqlErrors = &gqlErrors
	}

	return errResponse
}

// HTTPError is the error when the response status code is not 2xx.
type HTTPError struct {
	// Message is a human-readable summary that inlines the response body
	// truncated to maxErrorBodyLen. Use Body for the full payload.
	Message string `json:"message"`
	// Body is the full, untruncated response body.
	Body string `json:"body"`
	Code int    `json:"code"`
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http status %d: %s", e.Code, e.Message)
}

// maxErrorBodyLen bounds how many bytes of a non-2xx response body are inlined
// into HTTPError.Message, so a large or sensitive error page does not flood the
// error string and logs. The full body remains available in HTTPError.Body.
const maxErrorBodyLen = 1 << 10 // 1 KiB

// truncateForMessage shortens s to at most maxErrorBodyLen bytes on a UTF-8
// rune boundary, appending a note (with the full length) when truncated.
func truncateForMessage(s string) string {
	if len(s) <= maxErrorBodyLen {
		return s
	}

	cut := maxErrorBodyLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	return fmt.Sprintf("%s… (%d bytes total, truncated; full body in HTTPError.Body)", s[:cut], len(s))
}

// ErrorResponse represents a handled error of a GraphQL request.
type ErrorResponse struct {
	// http status code is not OK
	NetworkError *HTTPError `json:"networkErrors"`
	// http status code is OK but the server returned at least one graphql error
	GqlErrors *gqlerror.List `json:"graphqlErrors"`
}

// HasErrors returns true when at least one error is declared.
func (er *ErrorResponse) HasErrors() bool {
	return er.NetworkError != nil || er.GqlErrors != nil
}

func (er *ErrorResponse) Error() string {
	content, err := json.Marshal(er)
	if err != nil {
		return err.Error()
	}

	return string(content)
}

// Unwrap exposes the underlying errors so that callers can inspect them
// with errors.As, e.g. as *HTTPError or gqlerror.List.
func (er *ErrorResponse) Unwrap() []error {
	var errs []error
	if er.NetworkError != nil {
		errs = append(errs, er.NetworkError)
	}
	if er.GqlErrors != nil {
		errs = append(errs, *er.GqlErrors)
	}

	return errs
}

// unmarshalResponse decodes a GraphQL response body. It scans the top-level
// object once, capturing the raw "data" value and decoding the "errors" member
// into a gqlerror.List without reparsing the body. "data" is decoded into out
// only after the whole object is read, so a type mismatch in "data" never masks
// the "errors" the server returned alongside it: GraphQL errors are surfaced in
// preference to a data decode error, and partial data is still written to out.
func unmarshalResponse(respBody []byte, out any) error {
	dec := jsontext.NewDecoder(bytes.NewReader(respBody))

	tok, err := dec.ReadToken()
	if err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	if tok.Kind() != '{' {
		return fmt.Errorf("failed to decode response: unexpected JSON kind %v, expected object", tok.Kind())
	}

	var dataRaw jsontext.Value
	var errs gqlerror.List
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		switch name.String() {
		case "data":
			// ReadValue の戻り値は次の読み取りで無効化されうるためクローンして保持する。
			val, err := dec.ReadValue()
			if err != nil {
				return fmt.Errorf("failed to decode response data: %w", err)
			}
			dataRaw = append(dataRaw[:0], val...)
		case "errors":
			if err := json.UnmarshalDecode(dec, &errs); err != nil {
				return fmt.Errorf("failed to decode response error: %w", err)
			}
		default:
			if err := dec.SkipValue(); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
		}
	}

	// GraphQL errors があれば最優先で返す。data は best-effort で out に展開して
	// 部分データを保持する（型不一致でも errors をデコードエラーで握り潰さない）。
	if len(errs) > 0 {
		if dataRaw != nil {
			_ = json.Unmarshal(dataRaw, out)
		}

		return errs
	}

	if dataRaw == nil {
		return errors.New("failed to decode response: no data or errors member")
	}

	if err := json.Unmarshal(dataRaw, out); err != nil {
		return fmt.Errorf("failed to decode response data: %w", err)
	}

	return nil
}
