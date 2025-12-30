package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
)

func Test_multipartRequest(t *testing.T) {
	// Create a temporary directory using testing.T.TempDir()
	tempDir := t.TempDir()
	tempFilePath := filepath.Join(tempDir, "test.txt")

	// Create a temporary file
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer tempFile.Close()

	// Write content to the file
	if _, err := tempFile.WriteString("test content"); err != nil {
		t.Fatal(err)
	}

	if _, err := tempFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	// Create a second test file
	tempFilePath2 := filepath.Join(tempDir, "test2.txt")

	tempFile2, err := os.Create(tempFilePath2)
	if err != nil {
		t.Fatal(err)
	}

	defer tempFile2.Close()

	if _, err := tempFile2.WriteString("another test content"); err != nil {
		t.Fatal(err)
	}

	if _, err := tempFile2.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	// Common function to validate basic request properties
	validateRequest := func(r *http.Request) error {
		if r.Method != http.MethodPost {
			return fmt.Errorf("method got = %v want %v", r.Method, http.MethodPost)
		}

		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			return fmt.Errorf("content-Type got = %v, want to contain %v", r.Header.Get("Content-Type"), "multipart/form-data")
		}

		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return fmt.Errorf("parseMultipartForm got = %w, want nil", err)
		}

		return nil
	}

	// Verify operations field exists
	validateOperationsField := func(r *http.Request) error {
		if r.MultipartForm.Value["operations"] == nil {
			return errors.New("multipartForm field 'operations' not found")
		}

		return nil
	}

	// Verify map field exists
	validateMapField := func(r *http.Request) error {
		if r.MultipartForm.Value["map"] == nil {
			return errors.New("multipartForm field 'map' not found")
		}

		return nil
	}

	type args struct {
		ctx           context.Context
		variables     map[string]any
		endpoint      string
		operationName string
		query         string
	}

	tests := []struct {
		args       args
		name       string
		validators []func(*http.Request) error
		wantErr    bool
	}{
		{
			name: "File upload request",
			args: args{
				ctx:           t.Context(),
				endpoint:      "http://example.com/graphql",
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload!) { uploadFile(file: $file) }",
				variables: map[string]any{
					"file": graphql.Upload{
						File:     tempFile,
						Filename: "test.txt",
						Size:     12,
					},
				},
			},
			wantErr: false,
			validators: []func(*http.Request) error{
				validateOperationsField,
				validateMapField,
			},
		},
		{
			name: "Empty variables map case",
			args: args{
				ctx:           t.Context(),
				endpoint:      "http://example.com/graphql",
				operationName: "TestQuery",
				query:         "query TestQuery { test }",
				variables:     map[string]any{},
			},
			wantErr: false,
			validators: []func(*http.Request) error{
				validateOperationsField,
			},
		},
		{
			name: "Variables with nil pointer case",
			args: args{
				ctx:           t.Context(),
				endpoint:      "http://example.com/graphql",
				operationName: "UploadFile",
				query:         "mutation UploadFile($file: Upload) { uploadFile(file: $file) }",
				variables: map[string]any{
					"file": (*graphql.Upload)(nil),
				},
			},
			wantErr: false,
			validators: []func(*http.Request) error{
				validateOperationsField,
			},
		},
		{
			name: "Multiple files upload case",
			args: args{
				ctx:           t.Context(),
				endpoint:      "http://example.com/graphql",
				operationName: "UploadFiles",
				query:         "mutation UploadFiles($files: [Upload!]!) { uploadFiles(files: $files) }",
				variables: map[string]any{
					"files": []*graphql.Upload{
						{
							File:     tempFile,
							Filename: "test.txt",
							Size:     12,
						},
						{
							File:     tempFile2,
							Filename: "test2.txt",
							Size:     19,
						},
					},
				},
			},
			wantErr: false,
			validators: []func(*http.Request) error{
				validateOperationsField,
				validateMapField,
				func(r *http.Request) error {
					// Should have multiple files
					fileCount := len(r.MultipartForm.File)
					if fileCount != 2 {
						return fmt.Errorf("want 2 files, got %d", fileCount)
					}

					return nil
				},
			},
		},
		{
			name: "Empty file array case",
			args: args{
				ctx:           t.Context(),
				endpoint:      "http://example.com/graphql",
				operationName: "UploadFiles",
				query:         "mutation UploadFiles($files: [Upload!]) { uploadFiles(files: $files) }",
				variables: map[string]any{
					"files": []*graphql.Upload{},
				},
			},
			wantErr: false,
			validators: []func(*http.Request) error{
				validateOperationsField,
				func(r *http.Request) error {
					// Should have no files
					fileCount := 0
					if r.MultipartForm.File != nil {
						fileCount = len(r.MultipartForm.File)
					}

					if fileCount != 0 {
						return fmt.Errorf("want 0 files, got %d", fileCount)
					}

					return nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewMultipartRequest(tt.args.ctx, tt.args.endpoint, tt.args.operationName, tt.args.query, tt.args.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("multipartRequest() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if got != nil {
				// Execute basic validation first
				if err := validateRequest(got); err != nil {
					t.Errorf("multipartRequest() failed basic validation: %v", err)

					return
				}

				// Execute test case specific validations
				for i, validator := range tt.validators {
					if err := validator(got); err != nil {
						t.Errorf("multipartRequest() failed validator #%d: %v", i, err)

						return
					}
				}
			}
		})
	}
}
