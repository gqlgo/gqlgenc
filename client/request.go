package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	json "encoding/json/v2"
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
		"Content-Type": []string{"application/json;charset=utf-8"},
		"Accept":       []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
	}

	return req, nil
}
