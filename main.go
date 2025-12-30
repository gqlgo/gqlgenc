package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

const version = "3.0.0"

var versionOption = flag.Bool("version", false, "gqlgenc version")

func main() {
	flag.Parse()

	if *versionOption {
		fmt.Printf("gqlgenc v%s", version)

		return
	}

	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
