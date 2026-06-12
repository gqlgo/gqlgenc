package client

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"

	"encoding/json/jsontext"
	json "encoding/json/v2"

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

	errResponse := &errorResponse{}
	isStatusCodeOK := http.StatusOK <= resp.StatusCode && resp.StatusCode < http.StatusMultipleChoices
	if !isStatusCodeOK {
		errResponse.NetworkError = &httpError{
			Code:    resp.StatusCode,
			Message: "Response body " + string(body),
		}
	}

	if err := unmarshalResponse(body, out); err != nil {
		var gqlErrs *gqlErrors
		if errors.As(err, &gqlErrs) {
			// successfully parsed graphql error response
			errResponse.GqlErrors = &gqlErrs.Errors
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

// httpError is the error when a gqlErrors cannot be parsed.
type httpError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// gqlErrors is the struct of a standard graphql error response.
type gqlErrors struct {
	Errors gqlerror.List `json:"errors"`
}

func (e *gqlErrors) Error() string {
	return e.Errors.Error()
}

// errorResponse represent an handled error.
type errorResponse struct {
	// http status code is not OK
	NetworkError *httpError `json:"networkErrors"`
	// http status code is OK but the server returned at least one graphql error
	GqlErrors *gqlerror.List `json:"graphqlErrors"`
}

// HasErrors returns true when at least one error is declared.
func (er *errorResponse) HasErrors() bool {
	return er.NetworkError != nil || er.GqlErrors != nil
}

func (er *errorResponse) Error() string {
	content, err := json.Marshal(er)
	if err != nil {
		return err.Error()
	}

	return string(content)
}

// unmarshalResponse decodes a GraphQL response body in a single pass.
// The "data" member is streamed directly into out, and the "errors" member
// is decoded into a gqlerror.List without reparsing the body.
func unmarshalResponse(respBody []byte, out any) error {
	dec := jsontext.NewDecoder(bytes.NewReader(respBody))

	tok, err := dec.ReadToken()
	if err != nil {
		return fmt.Errorf("failed to decode response %q: %w", respBody, err)
	}
	if tok.Kind() != '{' {
		return fmt.Errorf("failed to decode response %q: unexpected JSON kind %v, expected object", respBody, tok.Kind())
	}

	var dataSeen bool
	var errs gqlerror.List
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("failed to decode response %q: %w", respBody, err)
		}
		switch name.String() {
		case "data":
			dataSeen = true
			if err := json.UnmarshalDecode(dec, out); err != nil {
				return fmt.Errorf("failed to decode response data %q: %w", respBody, err)
			}
		case "errors":
			if err := json.UnmarshalDecode(dec, &errs); err != nil {
				return fmt.Errorf("failed to decode response error %q: %w", respBody, err)
			}
		default:
			if err := dec.SkipValue(); err != nil {
				return fmt.Errorf("failed to decode response %q: %w", respBody, err)
			}
		}
	}

	if len(errs) > 0 {
		return &gqlErrors{Errors: errs}
	}

	if !dataSeen {
		return fmt.Errorf("failed to decode response %q: no data or errors member", respBody)
	}

	return nil
}
