package client

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"

	"encoding/json"
	"encoding/json/jsontext"

	"github.com/Yamashou/gqlgenc/v3/graphqljson"

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

// response is a GraphQL layer response from a handler.
type response struct {
	Data   jsontext.Value `json:"data"`
	Errors jsontext.Value `json:"errors"`
}

func unmarshalResponse(respBody []byte, out any) error {
	resp := response{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("failed to decode response %q: %w", respBody, err)
	}

	if err := graphqljson.UnmarshalData(resp.Data, out); err != nil {
		return fmt.Errorf("failed to decode response data %q: %w", resp.Data, err)
	}

	if len(resp.Errors) > 0 {
		gqlErrs := &gqlErrors{}
		if err := json.Unmarshal(respBody, gqlErrs); err != nil {
			return fmt.Errorf("failed to decode response error %q: %w", respBody, err)
		}

		return gqlErrs
	}

	return nil
}
