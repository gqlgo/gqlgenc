package client

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const (
	contentTypeJSON       = "application/json;charset=utf-8"
	acceptGraphQLResponse = "application/graphql-response+json;charset=utf-8"

	headerContentType = "Content-Type"
	headerAccept      = "Accept"
)

// Request represents an outgoing GraphQL request.
type Request struct {
	Query         string `json:"query"`
	Variables     any    `json:"variables,omitempty"`
	OperationName string `json:"operationName,omitempty"`
}

func NewRequest(ctx context.Context, endpoint, operationName, query string, variables any) (*http.Request, error) {
	graphqlRequest := &Request{
		Query:         query,
		Variables:     variables,
		OperationName: operationName,
	}

	uploads := &uploadCollector{}
	requestBody := &bytes.Buffer{}
	if err := json.MarshalWrite(requestBody, graphqlRequest, json.WithMarshalers(uploads.marshalers())); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	// variables に graphql.Upload が含まれる場合は multipart/form-data で送信する
	if len(uploads.files) > 0 {
		return newMultipartRequest(ctx, endpoint, requestBody.Bytes(), uploads)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, requestBody)
	if err != nil {
		return nil, fmt.Errorf("create request struct failed: %w", err)
	}

	req.Header = http.Header{
		headerContentType: []string{contentTypeJSON},
		headerAccept:      []string{acceptGraphQLResponse, contentTypeJSON},
	}

	return req, nil
}

// NewGetRequest builds a GraphQL-over-HTTP GET request, encoding query,
// operationName and variables into the URL query string. Variables containing
// graphql.Upload are rejected because a GET request has no body.
func NewGetRequest(ctx context.Context, endpoint, operationName, query string, variables any) (*http.Request, error) {
	uploads := &uploadCollector{}
	varsJSON, err := json.Marshal(variables, json.WithMarshalers(uploads.marshalers()))
	if err != nil {
		return nil, fmt.Errorf("encode variables: %w", err)
	}
	if len(uploads.files) > 0 {
		return nil, errors.New("GET requests do not support file uploads; use Post")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	if operationName != "" {
		q.Set("operationName", operationName)
	}
	if len(varsJSON) > 0 && string(varsJSON) != "null" {
		q.Set("variables", string(varsJSON))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request struct failed: %w", err)
	}

	req.Header = http.Header{
		headerAccept: []string{acceptGraphQLResponse, contentTypeJSON},
	}

	return req, nil
}
