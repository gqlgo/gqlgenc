package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Request represents an outgoing GraphQL request.
type Request struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

func NewRequest(ctx context.Context, endpoint, operationName, query string, variables map[string]any) (*http.Request, error) {
	graphqlRequest := &Request{
		Query:         query,
		Variables:     variables,
		OperationName: operationName,
	}

	requestBody, err := json.Marshal(graphqlRequest)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create request struct failed: %w", err)
	}

	req.Header = http.Header{
		"Content-Type": []string{"application/json;charset=utf-8"},
		"Accept":       []string{"application/graphql-response+json;charset=utf-8", "application/json;charset=utf-8"},
	}

	return req, nil
}
