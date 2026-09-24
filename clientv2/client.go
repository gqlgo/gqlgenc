package clientv2

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/99designs/gqlgen/graphql"

	"github.com/gqlgo/gqlgenc/graphqljson"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
	Post(url, contentType string, body io.Reader) (*http.Response, error)
}

type GQLRequestInfo struct {
	Request *Request
}

func NewGQLRequestInfo(r *Request) *GQLRequestInfo {
	return &GQLRequestInfo{
		Request: r,
	}
}

type RequestInterceptorFunc func(ctx context.Context, req *http.Request, gqlInfo *GQLRequestInfo, res any) error

type RequestInterceptor func(ctx context.Context, req *http.Request, gqlInfo *GQLRequestInfo, res any, next RequestInterceptorFunc) error

func ChainInterceptor(interceptors ...RequestInterceptor) RequestInterceptor {
	n := len(interceptors)

	return func(ctx context.Context, req *http.Request, gqlInfo *GQLRequestInfo, res any, next RequestInterceptorFunc) error {
		chainer := func(currentInter RequestInterceptor, currentFunc RequestInterceptorFunc) RequestInterceptorFunc {
			return func(currentCtx context.Context, currentReq *http.Request, currentGqlInfo *GQLRequestInfo, currentRes any) error {
				return currentInter(currentCtx, currentReq, currentGqlInfo, currentRes, currentFunc)
			}
		}

		chainedHandler := next
		for i := n - 1; i >= 0; i-- {
			chainedHandler = chainer(interceptors[i], chainedHandler)
		}

		return chainedHandler(ctx, req, gqlInfo, res)
	}
}

func UnsafeChainInterceptor(interceptors ...RequestInterceptor) RequestInterceptor {
	n := len(interceptors)

	return func(ctx context.Context, req *http.Request, gqlInfo *GQLRequestInfo, res any, next RequestInterceptorFunc) error {
		chainer := func(currentInter RequestInterceptor, currentFunc RequestInterceptorFunc) RequestInterceptorFunc {
			return func(currentCtx context.Context, currentReq *http.Request, currentGqlInfo *GQLRequestInfo, currentRes any) error {
				return currentInter(currentCtx, currentReq, currentGqlInfo, currentRes, func(nextCtx context.Context, nextReq *http.Request, nextGqlInfo *GQLRequestInfo, nextRes any) error {
					return currentFunc(nextCtx, nextReq, nextGqlInfo, nextRes)
				})
			}
		}

		chainedHandler := next
		for i := n - 1; i >= 0; i-- {
			chainedHandler = chainer(interceptors[i], chainedHandler)
		}

		return chainedHandler(ctx, req, gqlInfo, res)
	}
}

// Client is the http client wrapper
type Client struct {
	Client                     HttpClient
	BaseURL                    string
	RequestInterceptor         RequestInterceptor
	CustomDo                   RequestInterceptorFunc
	ParseDataWhenErrors        bool
	IsUnsafeRequestInterceptor bool
	EncodeNilSliceAsEmptyArray bool
}

// Request represents an outgoing GraphQL request
type Request struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

// NewClient creates a new http client wrapper
func NewClient(client HttpClient, baseURL string, options *Options, interceptors ...RequestInterceptor) *Client {
	c := &Client{
		Client:  client,
		BaseURL: baseURL,
		RequestInterceptor: ChainInterceptor(append([]RequestInterceptor{func(ctx context.Context, requestSet *http.Request, gqlInfo *GQLRequestInfo, res any, next RequestInterceptorFunc) error {
			return next(ctx, requestSet, gqlInfo, res)
		}}, interceptors...)...),
	}

	if options != nil {
		c.ParseDataWhenErrors = options.ParseDataAlongWithErrors
		c.EncodeNilSliceAsEmptyArray = options.EncodeNilSliceAsEmptyArray
	}

	return c
}

func NewClientWithUnsafeRequestInterceptor(client HttpClient, baseURL string, options *Options, interceptors ...RequestInterceptor) *Client {
	c := &Client{
		Client:  client,
		BaseURL: baseURL,
		RequestInterceptor: UnsafeChainInterceptor(append([]RequestInterceptor{func(ctx context.Context, requestSet *http.Request, gqlInfo *GQLRequestInfo, res any, next RequestInterceptorFunc) error {
			return next(ctx, requestSet, gqlInfo, res)
		}}, interceptors...)...),
		IsUnsafeRequestInterceptor: true,
	}

	if options != nil {
		c.ParseDataWhenErrors = options.ParseDataAlongWithErrors
		c.EncodeNilSliceAsEmptyArray = options.EncodeNilSliceAsEmptyArray
	}

	return c
}

// Options is a struct that holds some client-specific options that can be passed to NewClient.
type Options struct {
	// ParseDataAlongWithErrors is a flag that indicates whether the client should try to parse and return the data along with error
	// when error appeared. So in the end you'll get list of gql errors and data.
	ParseDataAlongWithErrors bool
	// EncodeNilSliceAsEmptyArray is a flag that indicates whether the client should encode nil slices
	// in request variables as [] instead of null. This restores the encoding behavior prior to
	// https://github.com/gqlgo/gqlgenc/pull/265, which is useful when a server schema declares
	// non-null list inputs (e.g. [Item!]!) that reject null.
	EncodeNilSliceAsEmptyArray bool
}

// GqlErrorList is the struct of a standard graphql error response
type GqlErrorList struct {
	Errors gqlerror.List `json:"errors"`
}

func (e *GqlErrorList) Error() string {
	return e.Errors.Error()
}

// HTTPError is the error when a GqlErrorList cannot be parsed
type HTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse represent an handled error
type ErrorResponse struct {
	// populated when http status code is not OK
	NetworkError *HTTPError `json:"networkErrors"`
	// populated when http status code is OK but the server returned at least one graphql error
	GqlErrors *gqlerror.List `json:"graphqlErrors"`
}

// HasErrors returns true when at least one error is declared
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

type MultipartFile struct {
	File  graphql.Upload
	Index int
}

type MultipartFilesGroup struct {
	Files      []MultipartFile
	IsMultiple bool
}

type FormField struct {
	Name  string
	Value any
}

// Post support send multipart form with files https://gqlgen.com/reference/file-upload/ https://github.com/jaydenseric/graphql-multipart-request-spec
func (c *Client) Post(ctx context.Context, operationName, query string, respData any, vars map[string]any, interceptors ...RequestInterceptor) error {
	multipartFilesGroups, mapping, vars := parseMultipartFiles(vars)

	r := &Request{
		Query:         query,
		Variables:     vars,
		OperationName: operationName,
	}

	gqlInfo := NewGQLRequestInfo(r)

	body, headers, err := c.requestBody(ctx, r, multipartFilesGroups, mapping)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, body)
	if err != nil {
		return fmt.Errorf("create request struct failed: %w", err)
	}

	maps.Copy(req.Header, headers)

	do := c.do
	// if custom do is set, use it instead of the default one
	if c.CustomDo != nil {
		do = c.CustomDo
	}

	return c.chainInterceptors(interceptors)(ctx, req, gqlInfo, respData, do)
}

// requestBody encodes the request as a multipart form when files are attached, otherwise as JSON.
//
// Arguments:
//   - ctx: passed to the encoder
//   - r: the request to send
//   - multipartFilesGroups: the files extracted from the variables
//   - mapping: the multipart map field, from file index to variable path
//
// Returns:
//   - io.Reader: the encoded body
//   - http.Header: the headers describing the body
//   - error: non-nil if the request cannot be encoded
//
// Preconditions:
//   - r.Variables no longer contains the files listed in multipartFilesGroups
//
// Postconditions:
//   - none
func (c *Client) requestBody(ctx context.Context, r *Request, multipartFilesGroups []MultipartFilesGroup, mapping map[string][]string) (io.Reader, http.Header, error) {
	headers := http.Header{}

	if len(multipartFilesGroups) > 0 {
		body := new(bytes.Buffer)

		contentType, err := c.prepareMultipartFormBody(
			ctx,
			body,
			[]FormField{
				{
					Name:  "operations",
					Value: r,
				},
				{
					Name:  "map",
					Value: mapping,
				},
			},
			multipartFilesGroups,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to prepare form body: %w", err)
		}

		headers.Set("Content-Type", contentType)

		return body, headers, nil
	}

	requestBody, err := c.marshalJSON(ctx, r)
	if err != nil {
		return nil, nil, fmt.Errorf("encode: %w", err)
	}

	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("Accept", "application/json; charset=utf-8")

	return bytes.NewBuffer(requestBody), headers, nil
}

// chainInterceptors combines the client interceptor with the per-request interceptors.
//
// Arguments:
//   - interceptors: the interceptors passed to Post, run after the client interceptor
//
// Returns:
//   - RequestInterceptor: the combined interceptor
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the unsafe chain is used when c.IsUnsafeRequestInterceptor is set
func (c *Client) chainInterceptors(interceptors []RequestInterceptor) RequestInterceptor {
	all := append([]RequestInterceptor{c.RequestInterceptor}, interceptors...)
	if c.IsUnsafeRequestInterceptor {
		return UnsafeChainInterceptor(all...)
	}

	return ChainInterceptor(all...)
}

func parseMultipartFiles(
	vars map[string]any,
) ([]MultipartFilesGroup, map[string][]string, map[string]any) {
	// Work on a copy: the caller may reuse its map, for example when a
	// request interceptor retries the request.
	vars = maps.Clone(vars)

	var (
		multipartFilesGroups []MultipartFilesGroup
		mapping              = map[string][]string{}
		i                    = 0
	)

	// addSingleUpload replaces the variable k with null and registers its file.
	addSingleUpload := func(k string, upload graphql.Upload) {
		iStr := strconv.Itoa(i)
		vars[k] = nil
		mapping[iStr] = []string{fmt.Sprintf("variables.%s", k)}

		multipartFilesGroups = append(multipartFilesGroups, MultipartFilesGroup{
			Files: []MultipartFile{
				{
					Index: i,
					File:  upload,
				},
			},
		})

		i++
	}

	for k, v := range vars {
		switch item := v.(type) {
		case graphql.Upload:
			addSingleUpload(k, item)
		case *graphql.Upload:
			// continue if it is empty
			if item == nil {
				continue
			}

			addSingleUpload(k, *item)
		case []*graphql.Upload:
			// Placeholders for the files; nil elements stay null in the operations body.
			placeholders := make([]any, len(item))
			vars[k] = placeholders

			groupFiles := make([]MultipartFile, 0, len(item))

			for itemI, itemV := range item {
				if itemV == nil {
					continue
				}

				placeholders[itemI] = struct{}{}

				iStr := strconv.Itoa(i)
				mapping[iStr] = []string{fmt.Sprintf("variables.%s.%s", k, strconv.Itoa(itemI))}

				groupFiles = append(groupFiles, MultipartFile{
					Index: i,
					File:  *itemV,
				})

				i++
			}

			multipartFilesGroups = append(multipartFilesGroups, MultipartFilesGroup{
				Files:      groupFiles,
				IsMultiple: true,
			})
		}
	}

	return multipartFilesGroups, mapping, vars
}

// prepareMultipartFormBody writes the multipart request body into buffer and
// returns its Content-Type. Form fields are encoded with the client encoder so
// that they follow the same rules as a JSON request body.
func (c *Client) prepareMultipartFormBody(
	ctx context.Context, buffer *bytes.Buffer, formFields []FormField, files []MultipartFilesGroup,
) (string, error) {
	writer := multipart.NewWriter(buffer)

	// form fields
	for _, field := range formFields {
		fieldBody, err := c.marshalJSON(ctx, field.Value)
		if err != nil {
			return "", fmt.Errorf("encode %s: %w", field.Name, err)
		}

		err = writer.WriteField(field.Name, string(fieldBody))
		if err != nil {
			return "", fmt.Errorf("write %s: %w", field.Name, err)
		}
	}

	// files
	for _, filesGroup := range files {
		for _, file := range filesGroup.Files {
			part, err := writer.CreateFormFile(strconv.Itoa(file.Index), file.File.Filename)
			if err != nil {
				return "", fmt.Errorf("form file %w", err)
			}

			_, err = io.Copy(part, file.File.File)
			if err != nil {
				return "", fmt.Errorf("copy file %w", err)
			}
		}
	}

	err := writer.Close()
	if err != nil {
		return "", fmt.Errorf("writer close %w", err)
	}

	return writer.FormDataContentType(), nil
}

func (c *Client) do(_ context.Context, req *http.Request, _ *GQLRequestInfo, res any) error {
	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") == "gzip" {
		resp.Body, err = gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("gzip decode failed: %w", err)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	return c.parseResponse(body, resp.StatusCode, res)
}

func (c *Client) parseResponse(body []byte, httpCode int, result any) error {
	errResponse := &ErrorResponse{}

	isErrorStatus := httpCode < 200 || 299 < httpCode
	if isErrorStatus {
		errResponse.NetworkError = &HTTPError{
			Code:    httpCode,
			Message: fmt.Sprintf("Response body %s", string(body)),
		}
	}

	// some servers return a graphql error with a non OK http code, try anyway to parse the body
	err := c.unmarshal(body, result)
	if err != nil {
		if gqlErr, ok := errors.AsType[*GqlErrorList](err); ok {
			errResponse.GqlErrors = &gqlErr.Errors
		} else if !isErrorStatus {
			return err
		}
	}

	if errResponse.HasErrors() {
		return errResponse
	}

	return nil
}

// response is a GraphQL layer response from a handler.
type response struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

func (c *Client) unmarshal(data []byte, res any) error {
	resp := response{}

	err := json.Unmarshal(data, &resp)
	if err != nil {
		return fmt.Errorf("failed to decode data %s: %w", string(data), err)
	}

	// gqlErrors is the standard GraphQL error list of the response, or nil.
	var gqlErrors error

	if len(resp.Errors) > 0 {
		// try to parse standard graphql error
		gqlErrorList := &GqlErrorList{}

		err = json.Unmarshal(data, gqlErrorList)
		if err != nil {
			return fmt.Errorf("faild to parse graphql errors. Response content %s - %w", string(data), err)
		}

		// if ParseDataWhenErrors is true, try to parse data as well
		if !c.ParseDataWhenErrors {
			return gqlErrorList
		}

		gqlErrors = gqlErrorList
	}

	errData := graphqljson.UnmarshalData(resp.Data, res)
	if errData != nil {
		// With ParseDataWhenErrors, data may be partial or null when the response
		// carries GraphQL errors, so report those errors instead of the decode
		// failure. Without GraphQL errors the decode failure is the only error.
		if c.ParseDataWhenErrors && gqlErrors != nil {
			return gqlErrors
		}

		return fmt.Errorf("failed to decode data into response %s: %w", string(data), errData)
	}

	return gqlErrors
}
