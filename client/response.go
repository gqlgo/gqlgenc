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

	"github.com/vektah/gqlparser/v2/gqlerror"
)

func ParseResponse(resp *http.Response, out any) error {
	if resp.Header.Get("Content-Encoding") == "gzip" {
		respBody, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to decode gzip: %w", err)
		}

		resp.Body = respBody
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	errResponse := &ErrorResponse{}
	isStatusCodeOK := http.StatusOK <= resp.StatusCode && resp.StatusCode < http.StatusMultipleChoices
	if !isStatusCodeOK {
		errResponse.NetworkError = &HTTPError{
			Code:    resp.StatusCode,
			Message: "Response body " + string(body),
		}
	}

	if err := unmarshalResponse(body, out); err != nil {
		if gqlErrs, ok := errors.AsType[gqlerror.List](err); ok {
			// successfully parsed graphql error response
			errResponse.GqlErrors = &gqlErrs
		} else if isStatusCodeOK {
			// status code is OK but the GraphQL response can't be parsed, it's an error.
			return fmt.Errorf("http status is OK but %w", err)
		}
	}

	if errResponse.HasErrors() {
		return errResponse
	}

	return nil
}

// HTTPError is the error when the response status code is not 2xx.
type HTTPError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http status %d: %s", e.Code, e.Message)
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

// unmarshalResponse decodes a GraphQL response body in a single pass.
// The "data" member is streamed directly into out, and the "errors" member
// is decoded into a gqlerror.List without reparsing the body.
func unmarshalResponse(respBody []byte, out any) error {
	dec := jsontext.NewDecoder(bytes.NewReader(respBody))

	tok, err := dec.ReadToken()
	if err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	if tok.Kind() != '{' {
		return fmt.Errorf("failed to decode response: unexpected JSON kind %v, expected object", tok.Kind())
	}

	var dataSeen bool
	var errs gqlerror.List
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		switch name.String() {
		case "data":
			dataSeen = true
			if err := json.UnmarshalDecode(dec, out); err != nil {
				return fmt.Errorf("failed to decode response data: %w", err)
			}
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

	if len(errs) > 0 {
		return errs
	}

	if !dataSeen {
		return errors.New("failed to decode response: no data or errors member")
	}

	return nil
}
