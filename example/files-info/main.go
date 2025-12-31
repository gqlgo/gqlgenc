package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql"
	"github.com/gqlgo/gqlgenc/clientv2"
	"github.com/gqlgo/gqlgenc/example/files-info/gen"
)

const FilesDir = "./example/files-info/files/"

const imageFile = "mario-strikers_1600w_original.jpg"

func newof[T any](val T) *T {
	return &val
}

func getFiles(files ...string) ([]*graphql.Upload, error) {
	result := make([]*graphql.Upload, len(files))

	for i, file := range files {
		uFile, err := NewUploadFile(file)
		if err != nil {
			return result, err
		}

		result[i] = newof(uFile)
	}

	return result, nil
}

func main() {
	client := NewFilesInfoClient(
		clientv2.NewClient(
			http.DefaultClient,
			"http://localhost:8080/query",
			nil,
		),
	)

	ctx := context.Background()
	files := []string{
		FilesDir + "color-bars-600.png",
		FilesDir + imageFile,
	}

	fmt.Println("Queries list")

	fmt.Println("Request: FileInfo")
	uFile, err := NewUploadFile(FilesDir + "color-bars-600.png")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fileInfoResp, err := client.FileInfo(ctx, uFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Response: %#+v\n\n", fileInfoResp)

	fmt.Println("Request: FilesInfo")
	uFiles, err := getFiles(files...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	filesInfoResp, err := client.FilesInfo(ctx, uFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Response: %#v\n\n", filesInfoResp)

	fmt.Println("Mutations list")

	fmt.Println("Request: UploadFile")

	uFile, err = NewUploadFile(FilesDir + imageFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	uploadFileResp, err := client.UploadFile(ctx, uFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Response: %#v\n\n", uploadFileResp)

	fmt.Println("Request: UploadFiles")

	uFiles, err = getFiles(files...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	uploadFilesResp, err := client.UploadFiles(ctx, uFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Response: %#v\n\n", uploadFilesResp)

	fmt.Println("Multiple queries in one request")

	fmt.Println("Request: AllFilesInfo")

	uFile, err = NewUploadFile(FilesDir + imageFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	uFiles, err = getFiles(files...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	allFilesInfoResp, err := client.AllFilesInfo(ctx, uFile, uFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	fmt.Printf("Response: %#v\n\n", allFilesInfoResp)

	fmt.Println("Request: AllFilesInfoWithListItems")

	uFile, err = NewUploadFile(FilesDir + imageFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	uFiles, err = getFiles(files...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	allFilesInfoWithListItemsResp, err := client.AllFilesInfoWithListItems(
		ctx,
		uFile,
		uFiles,
		&gen.ListItemsInput{IDAnyOf: []int{1, 3, 5}},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	fmt.Printf("Response: %#v\n\n", allFilesInfoWithListItemsResp)

	fmt.Println("Multiple mutations in one request")

	fmt.Println("Request: AllFilesInfo")

	uFile, err = NewUploadFile(FilesDir + imageFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	uFiles, err = getFiles(files...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	manyMutationsInOneResp, err := client.ManyMutationsInOne(ctx, uFile, uFiles, "New Item Name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err.Error())
		os.Exit(1)
	}

	fmt.Printf("Response: %#v\n\n", manyMutationsInOneResp)
}

func NewUploadFile(filePath string) (file graphql.Upload, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return file, fmt.Errorf("error: %s", err.Error())
	}

	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}()

	stat, err := f.Stat()
	if err != nil {
		return file, fmt.Errorf("stat error: %s", err.Error())
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return file, fmt.Errorf("read error: %s", err.Error())
	}

	file.File = bytes.NewReader(b)
	file.Filename = stat.Name()
	file.Size = stat.Size()
	file.ContentType = http.DetectContentType(b)

	return file, nil
}

func NewFilesInfoClient(c *clientv2.Client) *gen.Client {
	return &gen.Client{Client: c}
}
