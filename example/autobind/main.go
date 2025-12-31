package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gqlgo/gqlgenc/clientv2"
	"github.com/gqlgo/gqlgenc/example/github/gen"
)

func main() {
	// This example only read public repository. You don't need to select scopes.
	token := os.Getenv("GITHUB_TOKEN")
	ctx := context.Background()

	githubClient := &gen.Client{
		Client: clientv2.NewClient(http.DefaultClient, "https://api.github.com/graphql", nil,
			func(ctx context.Context, req *http.Request, gqlInfo *clientv2.GQLRequestInfo, res any, next clientv2.RequestInterceptorFunc) error {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

				return next(ctx, req, gqlInfo, res)
			}),
	}
	getUser, err := githubClient.GetUser(ctx, 10, 10)
	if err != nil {
		if handledError, ok := err.(*clientv2.ErrorResponse); ok {
			fmt.Fprintf(os.Stderr, "handled error: %s\n", handledError.Error())
		} else {
			fmt.Fprintf(os.Stderr, "unhandled error: %s\n", err.Error())
		}
		os.Exit(1)
	}

	fmt.Println(*getUser.Viewer.Name, getUser.Viewer.Repositories.Nodes[0].Name)
	for _, repository := range getUser.Viewer.Repositories.Nodes {
		fmt.Println(repository.Name)
		for _, language := range repository.Languages.Nodes {
			fmt.Println(language.Name)
		}

		res, err := githubClient.GetNode(ctx, repository.ID)
		if err != nil {
			if handledError, ok := err.(*clientv2.ErrorResponse); ok {
				fmt.Fprintf(os.Stderr, "handled error: %s\n", handledError.Error())
			} else {
				fmt.Fprintf(os.Stderr, "unhandled error: %s\n", err.Error())
			}
			os.Exit(1)
		}

		fmt.Println(res.Node.ID, res.Node.Repository.Name)
	}
}
